package agent

import (
	"crypto/md5"
	"strings"
)

// Color 定义代理颜色（使用 AgentColorName 的别名）
type Color = AgentColorName

// MemoryScope 本地别名
type MemoryScope = AgentMemoryScope

// ColorANSI 映射：颜色到ANSI转义序列
var colorANSI = map[Color]string{
	ColorBlue:    "\033[34m",
	ColorGreen:   "\033[32m",
	ColorRed:     "\033[31m",
	ColorYellow:  "\033[33m",
	ColorMagenta: "\033[35m",
	ColorCyan:    "\033[36m",
	ColorGray:    "\033[90m",
	ColorDefault: "\033[39m",
}

// ColorBackgroundANSI 映射：颜色到背景ANSI转义序列
var colorBackgroundANSI = map[Color]string{
	ColorBlue:    "\033[44m",
	ColorGreen:   "\033[42m",
	ColorRed:     "\033[41m",
	ColorYellow:  "\033[43m",
	ColorMagenta: "\033[45m",
	ColorCyan:    "\033[46m",
	ColorGray:    "\033[100m",
	ColorDefault: "\033[49m",
}

// ResetANSI 重置颜色
const ResetANSI = "\033[0m"

// IsValidColor 检查颜色是否有效
func IsValidColor(color Color) bool {
	for _, c := range ValidAgentColors {
		if c == color {
			return true
		}
	}
	return false
}

// GetColorANSI 获取颜色的前景ANSI码
func GetColorANSI(color Color) string {
	if code, ok := colorANSI[color]; ok {
		return code
	}
	return colorANSI[ColorDefault]
}

// GetBackgroundColorANSI 获取颜色的背景ANSI码
func GetBackgroundColorANSI(color Color) string {
	if code, ok := colorBackgroundANSI[color]; ok {
		return code
	}
	return colorBackgroundANSI[ColorDefault]
}

// Colorize 使用颜色包装文本
func Colorize(text string, color Color) string {
	if color == ColorDefault {
		return text
	}
	return GetColorANSI(color) + text + ResetANSI
}

// ColorizeBackground 使用背景色包装文本
func ColorizeBackground(text string, color Color) string {
	if color == ColorDefault {
		return text
	}
	return GetBackgroundColorANSI(color) + text + ResetANSI
}

// AgentColorManager 管理代理颜色分配
type AgentColorManager struct {
	assignedColors  map[string]Color
	availableColors []Color
	builtInColorMap map[string]Color
}

// NewAgentColorManager 创建新的颜色管理器
func NewAgentColorManager() *AgentColorManager {
	colors := make([]Color, 0, len(ValidAgentColors)-1)
	for _, c := range ValidAgentColors {
		if c != ColorDefault {
			colors = append(colors, c)
		}
	}

	builtInColors := map[string]Color{
		"explore":           ColorCyan,
		"plan":              ColorYellow,
		"claude-code-guide": ColorGreen,
		"verify":            ColorRed,
	}

	return &AgentColorManager{
		assignedColors:  make(map[string]Color),
		availableColors: colors,
		builtInColorMap: builtInColors,
	}
}

// GetAgentColor 获取代理的显示颜色
func (m *AgentColorManager) GetAgentColor(
	agentName string,
	agentDefinition interface{},
) Color {
	if agentDef, ok := agentDefinition.(interface{ GetColor() Color }); ok {
		color := agentDef.GetColor()
		if color != "" && IsValidColor(color) {
			m.assignedColors[agentName] = color
			return color
		}
	}

	if builtInColor, ok := m.builtInColorMap[strings.ToLower(agentName)]; ok {
		m.assignedColors[agentName] = builtInColor
		return builtInColor
	}

	if existingColor, ok := m.assignedColors[agentName]; ok {
		return existingColor
	}

	return m.assignNextColor(agentName)
}

// assignNextColor 分配下一个可用颜色
func (m *AgentColorManager) assignNextColor(agentName string) Color {
	if len(m.availableColors) == 0 {
		return ColorDefault
	}

	color := m.availableColors[0]
	m.availableColors = m.availableColors[1:]
	m.assignedColors[agentName] = color
	m.availableColors = append(m.availableColors, color)

	return color
}

// Reset 重置颜色管理器状态
func (m *AgentColorManager) Reset() {
	m.assignedColors = make(map[string]Color)
}

// GetAssignedColors 获取当前所有已分配的颜色
func (m *AgentColorManager) GetAssignedColors() map[string]Color {
	return m.assignedColors
}

// HasColor 检查是否已为某个代理分配了颜色
func (m *AgentColorManager) HasColor(agentName string) bool {
	_, ok := m.assignedColors[agentName]
	return ok
}

// ReleaseColor 释放代理的颜色
func (m *AgentColorManager) ReleaseColor(agentName string) {
	delete(m.assignedColors, agentName)
}

// GenerateColorFromName 从名称生成确定性的颜色
func GenerateColorFromName(name string) Color {
	hash := md5.Sum([]byte(name))
	colorIndex := hash[0] % byte(len(ValidAgentColors)-1)
	return ValidAgentColors[colorIndex]
}

// GetColorPalette 获取所有可用颜色
func GetColorPalette() []Color {
	return ValidAgentColors
}

// GetBrightColors 获取亮色列表
func GetBrightColors() []Color {
	return []Color{ColorCyan, ColorGreen, ColorYellow, ColorMagenta}
}

// GetDarkColors 获取暗色列表
func GetDarkColors() []Color {
	return []Color{ColorBlue, ColorRed, ColorGray}
}

// NeedsBackground 检查颜色是否需要背景色
func NeedsBackground(fg, bg Color) bool {
	return fg == bg
}

// CombineColors 组合前景和背景色
func CombineColors(fg, bg Color) string {
	if fg == ColorDefault && bg == ColorDefault {
		return ""
	}

	result := ""
	if bg != ColorDefault {
		result += GetBackgroundColorANSI(bg)
	}
	if fg != ColorDefault {
		result += GetColorANSI(fg)
	}
	return result
}
