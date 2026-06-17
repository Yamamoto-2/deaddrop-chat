package main

// Full-screen TUI that mirrors the web UI: a join form, then a responsive room
// (header + scrollable message log + input), adapting to the terminal size.
// Runs in the alternate screen and restores the terminal on exit, so it still
// leaves nothing behind (no-trace).

import (
	"fmt"
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
	}
	return m, nil
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
	in.Placeholder = "type a message…"
	in.CharLimit = 2000
	in.Focus()
	m.input = in
	m.vp = viewport.New(m.chatWidth(), m.chatHeight())
	m.refreshVP()

	m.scr = scrChat
	return tea.Batch(textinput.Blink, waitFor(m.incoming))
}

// --- chat screen ---

func (m *tuiModel) updateChat(msg tea.KeyMsg) (tea.Model, tea.Cmd) {
	switch msg.String() {
	case "esc":
		m.quit()
		return m, tea.Quit
	case "enter":
		text := m.input.Value()
		if strings.TrimSpace(text) != "" && m.client != nil {
			p := payload{Nick: m.myNick, Color: m.myColor, Ts: nowMs(), Text: text}
			_ = m.client.Send(p)
			m.appendMsg(p)
			m.input.SetValue("")
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
		m.appendMsg(msg.Msg)
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
	line := stDim.Render(clock(p.Ts)) + " " + nick + " " + stFg.Render(p.Text)
	m.lines = append(m.lines, line)
	m.refreshVP()
	m.vp.GotoBottom()
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
