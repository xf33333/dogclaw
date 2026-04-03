package ink

// Color represents a terminal color
type Color string

// ANSI color constants
const (
	ColorBlack         Color = "\033[30m"
	ColorRed           Color = "\033[31m"
	ColorGreen         Color = "\033[32m"
	ColorYellow        Color = "\033[33m"
	ColorBlue          Color = "\033[34m"
	ColorMagenta       Color = "\033[35m"
	ColorCyan          Color = "\033[36m"
	ColorWhite         Color = "\033[37m"
	ColorBlackBright   Color = "\033[90m"
	ColorRedBright     Color = "\033[91m"
	ColorGreenBright   Color = "\033[92m"
	ColorYellowBright  Color = "\033[93m"
	ColorBlueBright    Color = "\033[94m"
	ColorMagentaBright Color = "\033[95m"
	ColorCyanBright    Color = "\033[96m"
	ColorWhiteBright   Color = "\033[97m"
	ColorReset         Color = "\033[39m"
)

// Background color constants
const (
	BgBlack         Color = "\033[40m"
	BgRed           Color = "\033[41m"
	BgGreen         Color = "\033[42m"
	BgYellow        Color = "\033[43m"
	BgBlue          Color = "\033[44m"
	BgMagenta       Color = "\033[45m"
	BgCyan          Color = "\033[46m"
	BgWhite         Color = "\033[47m"
	BgBlackBright   Color = "\033[100m"
	BgRedBright     Color = "\033[101m"
	BgGreenBright   Color = "\033[102m"
	BgYellowBright  Color = "\033[103m"
	BgBlueBright    Color = "\033[104m"
	BgMagentaBright Color = "\033[105m"
	BgCyanBright    Color = "\033[106m"
	BgWhiteBright   Color = "\033[107m"
	BgReset         Color = "\033[49m"
)

// TextStyles represents text styling options
type TextStyles struct {
	Color           Color
	BackgroundColor Color
	Dim             bool
	Bold            bool
	Italic          bool
	Underline       bool
	Strikethrough   bool
	Inverse         bool
}

// Styles represents box/layout styling options
type Styles struct {
	// Text wrap style
	TextWrap string // "wrap", "truncate", "truncate-end", "truncate-middle", "truncate-start"

	// Position
	Position string      // "absolute", "relative"
	Top      interface{} // number or percent string
	Bottom   interface{}
	Left     interface{}
	Right    interface{}

	// Gap
	ColumnGap *int
	RowGap    *int
	Gap       *int

	// Margin
	Margin       *int
	MarginX      *int
	MarginY      *int
	MarginTop    *int
	MarginBottom *int
	MarginLeft   *int
	MarginRight  *int

	// Padding
	Padding       *int
	PaddingX      *int
	PaddingY      *int
	PaddingTop    *int
	PaddingBottom *int
	PaddingLeft   *int
	PaddingRight  *int

	// Flex
	FlexGrow       *int
	FlexShrink     *int
	FlexDirection  string // "row", "column", "row-reverse", "column-reverse"
	FlexBasis      interface{}
	FlexWrap       string // "nowrap", "wrap", "wrap-reverse"
	AlignItems     string // "flex-start", "center", "flex-end", "stretch"
	AlignSelf      string // "flex-start", "center", "flex-end", "auto"
	JustifyContent string // "flex-start", "center", "flex-end", "space-between", "space-around", "space-evenly"

	// Dimensions
	Width     interface{} // number or percent string
	Height    interface{}
	MinWidth  interface{}
	MinHeight interface{}
	MaxWidth  interface{}
	MaxHeight interface{}

	// Display
	Display string // "flex", "none"

	// Border
	BorderStyle       string
	BorderTop         *bool
	BorderBottom      *bool
	BorderLeft        *bool
	BorderRight       *bool
	BorderColor       Color
	BorderTopColor    Color
	BorderBottomColor Color
	BorderLeftColor   Color
	BorderRightColor  Color

	// Background
	BackgroundColor Color
	Opaque          bool

	// Overflow
	Overflow  string // "visible", "hidden", "scroll"
	OverflowX string
	OverflowY string

	// Selection
	NoSelect interface{} // bool or "from-left-edge"
}

// DefaultStyles returns a default empty style
func DefaultStyles() Styles {
	return Styles{}
}

// MergeStyles merges two styles, with b taking precedence
func MergeStyles(a, b Styles) Styles {
	result := a

	if b.TextWrap != "" {
		result.TextWrap = b.TextWrap
	}
	if b.Position != "" {
		result.Position = b.Position
	}
	if b.Top != nil {
		result.Top = b.Top
	}
	if b.Bottom != nil {
		result.Bottom = b.Bottom
	}
	if b.Left != nil {
		result.Left = b.Left
	}
	if b.Right != nil {
		result.Right = b.Right
	}
	if b.Gap != nil {
		result.Gap = b.Gap
		result.ColumnGap = b.Gap
		result.RowGap = b.Gap
	}
	if b.ColumnGap != nil {
		result.ColumnGap = b.ColumnGap
	}
	if b.RowGap != nil {
		result.RowGap = b.RowGap
	}
	if b.Margin != nil {
		result.Margin = b.Margin
		result.MarginX = b.Margin
		result.MarginY = b.Margin
	}
	if b.MarginX != nil {
		result.MarginX = b.MarginX
	}
	if b.MarginY != nil {
		result.MarginY = b.MarginY
	}
	if b.MarginTop != nil {
		result.MarginTop = b.MarginTop
	}
	if b.MarginBottom != nil {
		result.MarginBottom = b.MarginBottom
	}
	if b.MarginLeft != nil {
		result.MarginLeft = b.MarginLeft
	}
	if b.MarginRight != nil {
		result.MarginRight = b.MarginRight
	}
	if b.Padding != nil {
		result.Padding = b.Padding
		result.PaddingX = b.Padding
		result.PaddingY = b.Padding
	}
	if b.PaddingX != nil {
		result.PaddingX = b.PaddingX
	}
	if b.PaddingY != nil {
		result.PaddingY = b.PaddingY
	}
	if b.PaddingTop != nil {
		result.PaddingTop = b.PaddingTop
	}
	if b.PaddingBottom != nil {
		result.PaddingBottom = b.PaddingBottom
	}
	if b.PaddingLeft != nil {
		result.PaddingLeft = b.PaddingLeft
	}
	if b.PaddingRight != nil {
		result.PaddingRight = b.PaddingRight
	}
	if b.FlexGrow != nil {
		result.FlexGrow = b.FlexGrow
	}
	if b.FlexShrink != nil {
		result.FlexShrink = b.FlexShrink
	}
	if b.FlexDirection != "" {
		result.FlexDirection = b.FlexDirection
	}
	if b.FlexWrap != "" {
		result.FlexWrap = b.FlexWrap
	}
	if b.AlignItems != "" {
		result.AlignItems = b.AlignItems
	}
	if b.AlignSelf != "" {
		result.AlignSelf = b.AlignSelf
	}
	if b.JustifyContent != "" {
		result.JustifyContent = b.JustifyContent
	}
	if b.Width != nil {
		result.Width = b.Width
	}
	if b.Height != nil {
		result.Height = b.Height
	}
	if b.Display != "" {
		result.Display = b.Display
	}
	if b.BorderStyle != "" {
		result.BorderStyle = b.BorderStyle
	}
	if b.BackgroundColor != "" {
		result.BackgroundColor = b.BackgroundColor
	}
	if b.Overflow != "" {
		result.Overflow = b.Overflow
	}

	return result
}

// ApplyTextStyle applies text styling to a string
func ApplyTextStyle(text string, style TextStyles) string {
	var codes []string

	if style.Color != "" {
		codes = append(codes, string(style.Color))
	}
	if style.BackgroundColor != "" {
		codes = append(codes, string(style.BackgroundColor))
	}
	if style.Bold {
		codes = append(codes, "\033[1m")
	}
	if style.Dim {
		codes = append(codes, "\033[2m")
	}
	if style.Italic {
		codes = append(codes, "\033[3m")
	}
	if style.Underline {
		codes = append(codes, "\033[4m")
	}
	if style.Strikethrough {
		codes = append(codes, "\033[9m")
	}
	if style.Inverse {
		codes = append(codes, "\033[7m")
	}

	if len(codes) == 0 {
		return text
	}

	return joinCodes(codes) + text + string(ColorReset)
}

func joinCodes(codes []string) string {
	result := ""
	for _, c := range codes {
		result += c
	}
	return result
}
