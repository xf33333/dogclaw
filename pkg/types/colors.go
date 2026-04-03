package types

// AgentColorName 代理颜色名称
type AgentColorName string

const (
	ColorRed     AgentColorName = "red"
	ColorBlue    AgentColorName = "blue"
	ColorGreen   AgentColorName = "green"
	ColorYellow  AgentColorName = "yellow"
	ColorPurple  AgentColorName = "purple"
	ColorOrange  AgentColorName = "orange"
	ColorPink    AgentColorName = "pink"
	ColorCyan    AgentColorName = "cyan"
	ColorMagenta AgentColorName = "magenta"
	ColorGray    AgentColorName = "gray"
	ColorDefault AgentColorName = ""
)

// ValidAgentColors 有效的颜色列表
var ValidAgentColors = []AgentColorName{
	ColorRed, ColorBlue, ColorGreen, ColorYellow,
	ColorPurple, ColorOrange, ColorPink, ColorCyan,
	ColorMagenta, ColorGray,
}

// AgentMemoryScope 代理记忆作用域
type AgentMemoryScope string

const (
	MemoryUser    AgentMemoryScope = "user"    // ~/.dogclaw/agent-memory/
	MemoryProject AgentMemoryScope = "project" // .dogclaw/agent-memory/
	MemoryLocal   AgentMemoryScope = "local"   // .dogclaw/agent-memory-local/
)

// ValidMemoryScopes 有效的记忆作用域
var ValidMemoryScopes = []AgentMemoryScope{
	MemoryUser, MemoryProject, MemoryLocal,
}
