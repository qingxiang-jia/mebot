package main

import "github.com/egoist/mygo/ui"

// GNOME Symbolic Icons (16x16 standard grid, fill="currentColor")

var (
	// actions: list-add-symbolic
	IconAdd = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M7 2v5H2v2h5v5h2V9h5V7H9V2H7z"/></svg>`))

	// places: user-trash-symbolic
	IconTrash = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M6 1a1 1 0 0 0-1 1H2v2h1v9a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2V4h1V2h-3a1 1 0 0 0-1-1H6zm-1 3h6v9a1 1 0 0 1-1 1H6a1 1 0 0 1-1-1V4zm1 2v6h1V6H6zm3 0v6h1V6H9z"/></svg>`))

	// actions: document-open-symbolic
	IconImport = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M1 2a1 1 0 0 0-1 1v10a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1V5a1 1 0 0 0-1-1H7L5.5 2H1zm1 2h3l1.5 2H14v1.5L12 12H3L1.5 7.5V4z"/></svg>`))

	// mimetypes: text-x-generic-symbolic
	IconDocument = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M3 1a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V6l-5-5H3zm0 2h6v4h4v7a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V3a1 1 0 0 1 1-1zm2 5v1h6V8H5zm0 2v1h6v-1H5zm0 2v1h4v-1H5z"/></svg>`))

	// actions: document-save-symbolic
	IconExport = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M7 1v7H4.5L8 12.5 11.5 8H9V1H7zM2 11v3a1 1 0 0 0 1 1h10a1 1 0 0 0 1-1v-3h-2v2H4v-2H2z"/></svg>`))

	// status: weather-clear-symbolic
	IconSun = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M8 3.5a4.5 4.5 0 1 0 0 9 4.5 4.5 0 0 0 0-9zm0 1.5a3 3 0 1 1 0 6 3 3 0 0 1 0-6zM7.25 0v2.5h1.5V0h-1.5zm0 13.5V16h1.5v-2.5h-1.5zm-5-6.25H0v1.5h2.25v-1.5zm13.75 0h-2.25v1.5H16v-1.5zM2.87 2.16L1.81 3.22l1.77 1.77 1.06-1.06L2.87 2.16zm9.55 9.55l-1.06 1.06 1.77 1.77 1.06-1.06-1.77-1.77zM4.64 11.71l-1.77 1.77 1.06 1.06 1.77-1.77-1.06-1.06zm8.49-8.49l-1.77 1.77 1.06 1.06 1.77-1.77-1.06-1.06z"/></svg>`))

	// status: weather-clear-night-symbolic
	IconMoon = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M6.07 1.03a7 7 0 1 0 8.9 8.9 7.02 7.02 0 0 1-7.87-7.87c-.34-.34-.7-.68-1.03-1.03z"/></svg>`))

	// actions: edit-copy-symbolic
	IconCopy = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M4 1a2 2 0 0 0-2 2v8h2V3a1 1 0 0 1 1-1h7V1H4zm3 3a2 2 0 0 0-2 2v7a2 2 0 0 0 2 2h6a2 2 0 0 0 2-2V6a2 2 0 0 0-2-2H7zm0 2h6v7H7V6z"/></svg>`))

	// actions: edit-paste-symbolic
	IconPaste = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M6 1a1 1 0 0 0-1 1H3a2 2 0 0 0-2 2v10a2 2 0 0 0 2 2h10a2 2 0 0 0 2-2V4a2 2 0 0 0-2-2h-2a1 1 0 0 0-1-1H6zm0 2h4v1H6V3zM3 4h1v1a1 1 0 0 0 1 1h6a1 1 0 0 0 1-1V4h1a1 1 0 0 1 1 1v9a1 1 0 0 1-1 1H3a1 1 0 0 1-1-1V5a1 1 0 0 1 1-1z"/></svg>`))

	// actions: edit-select-all-symbolic
	IconSelectAll = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M1 2v2h2V2H1zm4 0v2h2V2H5zm4 0v2h2V2H9zm4 0v2h2V2h-2zM1 6v2h2V6H1zm12 0v2h2V6h-2zM1 10v2h2v-2H1zm12 0v2h2v-2h-2zM1 14v2h2v-2H1zm4 0v2h2v-2H5zm4 0v2h2v-2H9zm4 0v2h2v-2h-2zm-6.5-7.5L5 8l3 3 5-5-1.5-1.5L8 8 6.5 6.5z"/></svg>`))

	// actions: document-edit-symbolic
	IconEdit = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M11.013 1.427a1.75 1.75 0 0 1 2.474 0l1.086 1.086a1.75 1.75 0 0 1 0 2.474l-8.61 8.61a2 2 0 0 1-.947.534l-3.666.916a.75.75 0 0 1-.91-.91l.916-3.667a2 2 0 0 1 .534-.946l8.61-8.61.513.514zM12.427 2.487a.25.25 0 0 0-.354 0L11 3.56l2.44 2.44 1.073-1.073a.25.25 0 0 0 0-.354l-1.086-1.086zM12.38 7.06L9.94 4.62 3.284 11.275a.5.5 0 0 0-.134.237l-.612 2.448 2.448-.612a.5.5 0 0 0 .237-.134L12.38 7.06z"/></svg>`))

	// ui: check-box-outline-blank (Clear selection)
	IconSelectNone = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M3 3v10h10V3H3zm1.5 1.5h7v7h-7v-7z"/></svg>`))

	// actions: sparkle (AI summary)
	IconSparkle = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 16 16"><path fill="currentColor" d="M8 0l1.8 5.2L15 7l-5.2 1.8L8 14l-1.8-5.2L1 7l5.2-1.8L8 0zm0 2.4L6.9 5.6 3.7 7l3.2 1.4L8 11.6l1.1-3.2L12.3 7 9.1 5.6 8 2.4z"/></svg>`))

	// categories: preferences-system-symbolic (Settings)
	IconSettings = ui.MustParseSVG([]byte(`<svg xmlns="http://www.w3.org/2000/svg" width="16" height="16" viewBox="0 0 24 24"><path fill="currentColor" d="M19.14 12.94c.04-.3.06-.61.06-.94 0-.32-.02-.64-.07-.94l2.03-1.58c.18-.14.23-.41.12-.61l-1.92-3.32c-.12-.22-.37-.29-.59-.22l-2.39.96c-.5-.38-1.03-.7-1.62-.94l-.36-2.54c-.04-.24-.24-.41-.48-.41h-3.84c-.24 0-.43.17-.47.41l-.36 2.54c-.59.24-1.13.56-1.62.94l-2.39-.96c-.22-.08-.47 0-.59.22L2.74 8.87c-.12.22-.07.47.12.61l2.03 1.58c-.05.3-.09.63-.09.94s.02.64.07.94l-2.03 1.58c-.18.14-.23.41-.12.61l1.92 3.32c.12.22.37.29.59.22l2.39-.96c.5.38 1.03.7 1.62.94l.36 2.54c.05.24.24.41.48.41h3.84c.24 0 .44-.17.47-.41l.36-2.54c.59-.24 1.13-.56 1.62-.94l2.39.96c.22.08.47 0 .59-.22l1.92-3.32c.12-.22.07-.47-.12-.61l-2.01-1.58zM12 15.6c-1.98 0-3.6-1.62-3.6-3.6s1.62-3.6 3.6-3.6 3.6 1.62 3.6 3.6-1.62 3.6-3.6 3.6z"/></svg>`))
)
