package main

import "github.com/charmbracelet/lipgloss"

var (
	colorBackground = lipgloss.Color("#0B0D12")
	colorPanel      = lipgloss.Color("#11151D")
	colorRaised     = lipgloss.Color("#181D27")
	colorBorder     = lipgloss.Color("#293141")
	colorAccent     = lipgloss.Color("#B8F36B")
	colorAccentDim  = lipgloss.Color("#26371E")
	colorText       = lipgloss.Color("#EDF2F7")
	colorMuted      = lipgloss.Color("#7C8798")
	colorSubtle     = lipgloss.Color("#4A5568")
	colorDanger     = lipgloss.Color("#FF6B7A")
	colorWarning    = lipgloss.Color("#F4C95D")
	colorSuccess    = lipgloss.Color("#70E1A1")

	headerStyle = lipgloss.NewStyle().
			Background(colorPanel).
			Foreground(colorText).
			Padding(0, 0)

	footerStyle = lipgloss.NewStyle().
			Background(colorPanel).
			Foreground(colorMuted).
			Padding(0, 0)

	badgeStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Background(colorAccentDim).
			Bold(true).
			Padding(0, 1)

	toastStyle = lipgloss.NewStyle().
			Foreground(colorBackground).
			Background(colorAccent).
			Bold(true).
			Padding(0, 1)

	sidebarStyle = lipgloss.NewStyle().
			Background(colorPanel).
			Padding(1, 2)

	dividerStyle = lipgloss.NewStyle().Foreground(colorBorder)

	navItem = lipgloss.NewStyle().
		Foreground(colorMuted).
		Padding(0, 1)

	navActive = lipgloss.NewStyle().
			Foreground(colorAccent).
			Background(colorAccentDim).
			Bold(true).
			Padding(0, 1)

	sectionTitle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Bold(true)

	pageEyebrow = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	pageTitle = lipgloss.NewStyle().
			Foreground(colorText).
			Bold(true).
			MarginTop(1)

	pageSubtitle = lipgloss.NewStyle().
			Foreground(colorMuted).
			MarginTop(1)

	bodyStyle = lipgloss.NewStyle().Foreground(colorText)
	hintStyle = lipgloss.NewStyle().Foreground(colorMuted)

	searchBoxStyle = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorRaised).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Padding(0, 1).
			MarginTop(1).
			MarginBottom(1)

	cardStyle = lipgloss.NewStyle().
			Background(colorPanel).
			Border(lipgloss.NormalBorder(), false, false, false, true).
			BorderForeground(colorAccent).
			Padding(1, 1).
			MarginRight(1)

	cardTitle = lipgloss.NewStyle().Foreground(colorText).Bold(true)

	keycapStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Background(colorAccentDim).
			Bold(true).
			Padding(0, 1)

	listItem = lipgloss.NewStyle().
			Foreground(colorMuted).
			Padding(0, 1)

	listSelected = lipgloss.NewStyle().
			Foreground(colorText).
			Background(colorRaised).
			Bold(true).
			Padding(0, 1)

	emptyStyle = lipgloss.NewStyle().
			Foreground(colorMuted).
			Background(colorPanel).
			Border(lipgloss.RoundedBorder()).
			BorderForeground(colorBorder).
			Align(lipgloss.Center).
			Padding(2, 3)

	commandStyle = lipgloss.NewStyle().
			Foreground(colorAccent).
			Background(colorAccentDim).
			Padding(0, 1).
			MarginTop(1)

	logStyle      = lipgloss.NewStyle().Foreground(colorMuted)
	logErrorStyle = lipgloss.NewStyle().Foreground(colorDanger)

	paneEyebrow = lipgloss.NewStyle().
			Foreground(colorAccent).
			Bold(true)

	paneTitle = lipgloss.NewStyle().
			Foreground(colorText).
			Bold(true).
			MarginTop(1).
			MarginBottom(1)

	mainStyle = lipgloss.NewStyle().
			Background(colorBackground).
			Padding(1, 3)

	interactionPaneStyle = lipgloss.NewStyle().
				Foreground(colorText).
				Background(colorPanel).
				Border(lipgloss.NormalBorder(), false, false, false, true).
				BorderForeground(colorAccent).
				Padding(1, 2)

	contextPanelStyle  = lipgloss.NewStyle().Background(colorPanel).Padding(1, 2)
	contextTitleStyle  = lipgloss.NewStyle().Foreground(colorMuted).Bold(true)
	contextLineStyle   = lipgloss.NewStyle().Foreground(colorSubtle)
	stageDoneStyle     = lipgloss.NewStyle().Foreground(colorSuccess).Bold(true)
	stageActiveStyle   = lipgloss.NewStyle().Foreground(colorAccent).Bold(true)
	stageErrorStyle    = lipgloss.NewStyle().Foreground(colorDanger).Bold(true)
	stageDetailStyle   = lipgloss.NewStyle().Foreground(colorMuted)
	workspaceHintStyle = lipgloss.NewStyle().Foreground(colorMuted).Italic(true)
)
