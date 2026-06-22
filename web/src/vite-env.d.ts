// Side-effect CSS imports (e.g. `import "./styles/global.css"`) carry no types.
// Vite handles them at build time; this ambient declaration lets tsc resolve
// the import. TS 6.0 enforces this (TS2882) where 5.x silently allowed it.
declare module "*.css";
