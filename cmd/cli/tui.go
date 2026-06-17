package main

// Full-screen TUI that mirrors the web UI: a join form, then a responsive room
// (header + scrollable message log + input), adapting to the terminal size.
// Runs in the alternate screen and restores the terminal on exit, so it still
// leaves nothing behind (no-trace).

import (
	"encoding/base64"
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/textinput"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

var (
	cAccent = lipgloss.Color("#5fd7a7")
	cDim    = lipgloss.Color("#5f6f66")
	cFg     = lipgloss.Color("#d7e0da")

	stAccent = lipgloss.NewStyle().Foreground(cAccent)
	stDim    = lipgloss.NewStyle().Foreground(cDim)
	stFg     = lipgloss.NewStyle().Foreground(cFg)
)

// Roster presence cadence — must match the web client (proto.ts).
const (
	helloInterval = 15 * time.Second
	rosterExpire  = 45 * time.Second
)

// rosterPeer is one known room member, learned from encrypted hello beacons.
type rosterPeer struct {
	nick, color string
	last        time.Time
}

func rosterKey(nick, color string) string { return nick + " " + color }

// tickMsg drives the roster heartbeat.
type tickMsg struct{}

func heartbeatCmd() tea.Cmd {
	return tea.Tick(helloInterval, func(time.Time) tea.Msg { return tickMsg{} })
}

type screen int

const (
	scrJoin screen = iota
	scrChat
)

// wsMsg wraps an Incoming event for the bubbletea loop.
type wsMsg Incoming

func waitFor(ch chan Incoming) tea.Cmd {
	return func() tea.Msg {
		in, ok := <-ch
		if !ok {
			return wsMsg{Kind: "closed"}
		}
		return wsMsg(in)
	}
}

type tuiModel struct {
	server string
	scr    screen
	w, h   int

	// join form
	name, room, pass textinput.Model
	focus            int // 0 name, 1 room, 2 pass, 3 color
	colorIdx         int
	errMsg           string

	// chat
	client   *Client
	incoming chan Incoming
	vp       viewport.Model
	input    textinput.Model
	lines    []string
	myNick   string
	myColor  string
	roomName string
	online   int
	live     bool
	files    []filePayload         // received files, in arrival order (1-based to the user)
	roster   map[string]rosterPeer // who's in the room, from encrypted beacons
}

func runTUI(server, room, pass, nick string) {
	m := newTUIModel(server, room, pass, nick)
	if _, err := tea.NewProgram(&m, tea.WithAltScreen()).Run(); err != nil {
		fmt.Println("tui error:", err)
	}
}

func newTUIModel(server, room, pass, nick string) tuiModel {
	mk := func(ph, val string) textinput.Model {
		t := textinput.New()
		t.Prompt = "› "
		t.Placeholder = ph
		t.SetValue(val)
		return t
	}
	name := mk("name", nick)
	name.CharLimit = 32
	rm := mk("room name", room)
	rm.CharLimit = 64
	pw := mk("optional password", pass)
	pw.CharLimit = 128
	pw.EchoMode = textinput.EchoPassword
	pw.EchoCharacter = '•'

	m := tuiModel{
		server:   server,
		scr:      scrJoin,
		name:     name,
		room:     rm,
		pass:     pw,
		colorIdx: randInt(len(palette)),
	}
	m.setFocus(0)
	return m
}

func (m *tuiModel) Init() tea.Cmd { return textinput.Blink }

func (m *tuiModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.w, m.h = msg.Width, msg.Height
		m.layout()
		return m, nil
	case tea.KeyMsg:
		if msg.String() == "ctrl+c" {
			m.quit()
			return m, tea.Quit
		}
		if m.scr == scrJoin {
			return m.updateJoin(msg)
		}
		return m.updateChat(msg)
	case wsMsg:
		return m.updateWS(msg)
	case tickMsg:
		return m.updateTick()
	}
	return m, nil
}

// updateTick fires on the heartbeat: re-announce ourselves, expire stale peers,
// and reschedule — but stop once the connection is gone.
func (m *tuiModel) updateTick() (tea.Model, tea.Cmd) {
	if !m.live {
		return m, nil
	}
	m.pruneRoster()
	return m, tea.Batch(m.announce("hello"), heartbeatCmd())
}

// --- join screen ---

func (m *tuiModel) setFocus(i int) {
	m.focus = i
	m.name.Blur()
	m.room.Blur()
	m.pass.Blur()
	switch i {
	case 0:
		m.name.Focus()
	case 1:
		m.room.Focus()
	case 2:
		m.pass.Focus()
	}
}

func (m *tuiModel) updateJoin(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		return m, tea.Quit
	case "enter":
		return m, m.join()
	case "tab", "down":
		m.setFocus((m.focus + 1) % 4)
		return m, textinput.Blink
	case "shift+tab", "up":
		m.setFocus((m.focus + 3) % 4)
		return m, textinput.Blink
	case "left":
		if m.focus == 3 {
			m.colorIdx = (m.colorIdx - 1 + len(palette)) % len(palette)
		}
		return m, nil
	case "right":
		if m.focus == 3 {
			m.colorIdx = (m.colorIdx + 1) % len(palette)
		}
		return m, nil
	}
	var cmd tea.Cmd
	switch m.focus {
	case 0:
		m.name, cmd = m.name.Update(msg)
	case 1:
		m.room, cmd = m.room.Update(msg)
	case 2:
		m.pass, cmd = m.pass.Update(msg)
	}
	return m, cmd
}

func (m *tuiModel) join() tea.Cmd {
	room := strings.TrimSpace(m.room.Value())
	if room == "" {
		m.setFocus(1)
		return nil
	}
	nick := strings.TrimSpace(m.name.Value())
	if nick == "" {
		nick = "anon"
	}
	c, err := Dial(m.server, room, m.pass.Value()) // derives key (brief pause)
	if err != nil {
		m.errMsg = err.Error()
		return nil
	}
	m.client = c
	m.incoming = make(chan Incoming, 32)
	go c.ReadLoop(m.incoming)

	m.myNick = nick
	m.myColor = palette[m.colorIdx]
	m.roomName = room
	m.online = 1
	m.live = true
	m.lines = nil

	in := textinput.New()
	in.Prompt = "› "
	in.Placeholder = "message · /who · /save [N] · /sendfile <path> · /help"
	in.CharLimit = 2000
	in.Focus()
	m.input = in
	m.vp = viewport.New(m.chatWidth(), m.chatHeight())
	m.refreshVP()

	m.roster = map[string]rosterPeer{}
	m.touchPeer(m.myNick, m.myColor)

	m.scr = scrChat
	return tea.Batch(textinput.Blink, waitFor(m.incoming), m.announce("hello"), heartbeatCmd())
}

// --- chat screen ---

func (m *tuiModel) updateChat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.quit()
		return m, tea.Quit
	case "enter":
		text := m.input.Value()
		trimmed := strings.TrimSpace(text)
		if trimmed == "" {
			return m, nil
		}
		switch {
		case trimmed == "/who" || trimmed == "/names":
			m.showWho()
			m.input.SetValue("")
		case trimmed == "/help" || trimmed == "/?":
			m.showHelp()
			m.input.SetValue("")
		case trimmed == "/save" || strings.HasPrefix(trimmed, "/save "):
			m.handleSave(trimmed)
			m.input.SetValue("")
		case strings.HasPrefix(trimmed, "/sendfile"):
			m.handleSendFile(trimmed)
			m.input.SetValue("")
		default:
			if m.client != nil {
				p := payload{Nick: m.myNick, Color: m.myColor, Ts: nowMs(), Text: text}
				_ = m.client.Send(p)
				m.appendMsg(p)
				m.input.SetValue("")
			}
		}
		return m, nil
	case "pgup", "pgdown":
		var cmd tea.Cmd
		m.vp, cmd = m.vp.Update(msg)
		return m, cmd
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	return m, cmd
}

func (m *tuiModel) updateWS(msg wsMsg) (tea.Model, tea.Cmd) {
	switch msg.Kind {
	case "msg":
		p := msg.Msg
		switch p.Kind {
		case "hello":
			_, known := m.roster[rosterKey(p.Nick, p.Color)]
			m.touchPeer(p.Nick, p.Color)
			if !known {
				// reply so the newcomer learns about us too
				return m, tea.Batch(m.announce("hello"), waitFor(m.incoming))
			}
		case "bye":
			delete(m.roster, rosterKey(p.Nick, p.Color))
		default:
			m.appendMsg(p)
		}
	case "presence":
		m.online = msg.N
	case "closed":
		m.live = false
		return m, nil
	}
	return m, waitFor(m.incoming)
}

func (m *tuiModel) appendMsg(p payload) {
	color := p.Color
	if color == "" {
		color = "#9fb0a6"
	}
	nick := lipgloss.NewStyle().Foreground(lipgloss.Color(color)).Render(p.Nick + ":")
	prefix := stDim.Render(clock(p.Ts)) + " " + nick + " "
	if strings.TrimSpace(p.Text) != "" {
		m.lines = append(m.lines, prefix+stFg.Render(p.Text))
	}
	if p.File != nil {
		m.files = append(m.files, *p.File)
		n := len(m.files)
		info := fmt.Sprintf("📎 [file %d] %s · %s  (/save %d)", n, p.File.Name, humanSize(p.File.Size), n)
		m.lines = append(m.lines, prefix+stAccent.Render(info))
	}
	m.refreshVP()
	m.vp.GotoBottom()
}

// appendSys adds a local-only status line (never sent to anyone).
func (m *tuiModel) appendSys(text string) {
	m.lines = append(m.lines, stDim.Render("· "+text))
	m.refreshVP()
	m.vp.GotoBottom()
}

// --- roster (encrypted presence) ---

func (m *tuiModel) touchPeer(nick, color string) {
	if m.roster == nil {
		m.roster = map[string]rosterPeer{}
	}
	m.roster[rosterKey(nick, color)] = rosterPeer{nick: nick, color: color, last: time.Now()}
}

// announce sends an encrypted hello/bye beacon (in the background so it never
// blocks the UI loop) and updates our own roster entry.
func (m *tuiModel) announce(kind string) tea.Cmd {
	if m.client == nil {
		return nil
	}
	if kind == "hello" {
		m.touchPeer(m.myNick, m.myColor)
	} else {
		delete(m.roster, rosterKey(m.myNick, m.myColor))
	}
	c := m.client
	p := payload{Nick: m.myNick, Color: m.myColor, Ts: nowMs(), Kind: kind}
	return func() tea.Msg {
		_ = c.Send(p)
		return nil
	}
}

func (m *tuiModel) pruneRoster() {
	cutoff := time.Now().Add(-rosterExpire)
	for k, v := range m.roster {
		self := v.nick == m.myNick && v.color == m.myColor
		if v.last.Before(cutoff) && !self {
			delete(m.roster, k)
		}
	}
}

func (m *tuiModel) showWho() {
	m.pruneRoster()
	names := make([]string, 0, len(m.roster))
	for _, p := range m.roster {
		label := p.nick
		if p.nick == m.myNick && p.color == m.myColor {
			label += " (you)"
		}
		names = append(names, label)
	}
	sort.Strings(names)
	m.appendSys(fmt.Sprintf("in #%s (%d): %s", m.roomName, len(names), strings.Join(names, ", ")))
}

func (m *tuiModel) showHelp() {
	for _, line := range []string{
		"commands:",
		"  /who               who's in the room",
		"  /save [N] [path]   save a received file (default: most recent)",
		"  /sendfile <path>   send a file",
		"  /help              this help",
		"  Enter send · PgUp/PgDn scroll · Esc or Ctrl-C quit",
	} {
		m.appendSys(line)
	}
}

// handleSave parses "/save", "/save N", or "/save N path" and writes a received
// file to disk. Nothing is written unless the user runs this — the client stays
// no-trace by default.
func (m *tuiModel) handleSave(line string) {
	rest := strings.TrimSpace(strings.TrimPrefix(line, "/save"))
	idx := len(m.files) // default: most recent file
	dest := ""
	if rest != "" {
		parts := strings.SplitN(rest, " ", 2)
		if n, err := strconv.Atoi(parts[0]); err == nil {
			idx = n
			if len(parts) == 2 {
				dest = strings.TrimSpace(parts[1])
			}
		} else {
			dest = rest // no numeric index given; treat the whole arg as a path
		}
	}

	if len(m.files) == 0 {
		m.appendSys("no files received yet")
		return
	}
	if idx < 1 || idx > len(m.files) {
		m.appendSys(fmt.Sprintf("no file #%d (have 1..%d)", idx, len(m.files)))
		return
	}
	f := m.files[idx-1]
	data, err := base64.StdEncoding.DecodeString(f.Data)
	if err != nil {
		m.appendSys("save failed: corrupt file data")
		return
	}

	// The name is sender-controlled — collapse it to a bare basename so a
	// malicious peer can't path-traverse out of the chosen directory.
	safe := filepath.Base(filepath.FromSlash(f.Name))
	if safe == "." || safe == ".." || safe == "" || safe == string(filepath.Separator) {
		safe = "file"
	}
	target := safe
	if dest != "" {
		dest = expandHome(dest)
		if info, statErr := os.Stat(dest); statErr == nil && info.IsDir() {
			target = filepath.Join(dest, safe)
		} else {
			target = dest
		}
	}
	target = uniquePath(target)
	if err := os.WriteFile(target, data, 0o600); err != nil {
		m.appendSys("save failed: " + err.Error())
		return
	}
	m.appendSys("saved " + target)
}

// handleSendFile parses "/sendfile <path>", reads the file, and sends it as an
// encrypted attachment (same wire format as the web client).
func (m *tuiModel) handleSendFile(line string) {
	path := strings.TrimSpace(strings.TrimPrefix(line, "/sendfile"))
	if path == "" {
		m.appendSys("usage: /sendfile <path>")
		return
	}
	path = expandHome(path)
	data, err := os.ReadFile(path)
	if err != nil {
		m.appendSys("send failed: " + err.Error())
		return
	}
	if int64(len(data)) > maxFileBytes {
		m.appendSys(fmt.Sprintf("send failed: file too large (max %s)", humanSize(maxFileBytes)))
		return
	}
	mtype := mime.TypeByExtension(filepath.Ext(path))
	if mtype == "" {
		mtype = "application/octet-stream"
	}
	p := payload{
		Nick:  m.myNick,
		Color: m.myColor,
		Ts:    nowMs(),
		File: &filePayload{
			Name: filepath.Base(path),
			Mime: mtype,
			Size: int64(len(data)),
			Data: base64.StdEncoding.EncodeToString(data),
		},
	}
	if m.client == nil {
		m.appendSys("send failed: not connected")
		return
	}
	if err := m.client.Send(p); err != nil {
		m.appendSys("send failed: " + err.Error())
		return
	}
	m.appendMsg(p)
}

func (m *tuiModel) refreshVP() {
	w := m.vp.Width
	if w < 1 {
		w = 76
	}
	content := lipgloss.NewStyle().Width(w).Render(strings.Join(m.lines, "\n"))
	m.vp.SetContent(content)
}

// --- layout + view ---

func (m *tuiModel) chatWidth() int {
	if m.w < 10 {
		return 76
	}
	return m.w
}

func (m *tuiModel) chatHeight() int {
	h := m.h - 4 // header + 2 rules + input
	if h < 3 {
		h = 3
	}
	return h
}

func (m *tuiModel) layout() {
	if m.scr != scrChat {
		return
	}
	m.vp.Width = m.chatWidth()
	m.vp.Height = m.chatHeight()
	m.input.Width = max(10, m.w-4)
	m.refreshVP()
	m.vp.GotoBottom()
}

func (m *tuiModel) quit() {
	if m.client != nil {
		// best-effort "bye" so peers drop us from their roster immediately
		_ = m.client.Send(payload{Nick: m.myNick, Color: m.myColor, Ts: nowMs(), Kind: "bye"})
		m.client.Close()
	}
}

func (m *tuiModel) View() string {
	if m.scr == scrJoin {
		return m.viewJoin()
	}
	return m.viewChat()
}

func (m *tuiModel) viewJoin() string {
	var swatches strings.Builder
	for i, c := range palette {
		blk := lipgloss.NewStyle().Foreground(lipgloss.Color(c)).Render("██")
		if i == m.colorIdx {
			if m.focus == 3 {
				swatches.WriteString(stFg.Render("[") + blk + stFg.Render("]"))
			} else {
				swatches.WriteString(stDim.Render("[") + blk + stDim.Render("]"))
			}
		} else {
			swatches.WriteString(" " + blk + " ")
		}
	}

	rows := []string{
		stAccent.Render("▚ DEADDROP"),
		stDim.Render("anonymous · ephemeral · end-to-end encrypted"),
		"",
		stDim.Render("name ") + m.name.View(),
		stDim.Render("room ") + m.room.View(),
		stDim.Render("pass ") + m.pass.View(),
		stDim.Render("color ") + swatches.String(),
		"",
		stDim.Render("Tab/↑↓ move · ←→ color · Enter join · Esc quit"),
	}
	if m.errMsg != "" {
		rows = append(rows, lipgloss.NewStyle().Foreground(lipgloss.Color("#ff6b6b")).Render(m.errMsg))
	}
	body := lipgloss.JoinVertical(lipgloss.Left, rows...)
	if m.w == 0 || m.h == 0 {
		return body
	}
	return lipgloss.Place(m.w, m.h, lipgloss.Center, lipgloss.Center, body)
}

func (m *tuiModel) viewChat() string {
	left := stAccent.Render("▚ #" + m.roomName)
	status := "offline"
	statusStyle := stDim
	if m.live {
		status = "● live"
		statusStyle = stAccent
	}
	right := stDim.Render(fmt.Sprintf("◍ %d", m.online)) +
		stDim.Render(" · ") + statusStyle.Render(status) +
		stDim.Render(" · ") +
		lipgloss.NewStyle().Foreground(lipgloss.Color(orFg(m.myColor))).Render("● "+m.myNick)

	gap := m.w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}
	header := left + strings.Repeat(" ", gap) + right
	rule := stDim.Render(strings.Repeat("─", max(0, m.w)))

	return strings.Join([]string{
		header,
		rule,
		m.vp.View(),
		rule,
		m.input.View(),
	}, "\n")
}

func clock(ts int64) string {
	t := time.Now()
	if ts > 0 {
		t = time.UnixMilli(ts)
	}
	return fmt.Sprintf("%02d:%02d", t.Hour(), t.Minute())
}

func orFg(c string) string {
	if c == "" {
		return "#d7e0da"
	}
	return c
}
