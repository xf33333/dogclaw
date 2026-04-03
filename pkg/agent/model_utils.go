package agent

import (
	"os"
	"strings"

	"dogclaw/pkg/types"
)

// ========== Agent Model 相关（参考 utils/model/agent.ts） ==========

// AgentModelOption 代理模型选项
type AgentModelOption struct {
	Value       string
	Label       string
	Description string
}

// AgentModelOptions 有效的代理模型选项
var AgentModelOptions = []AgentModelOption{
	{Value: "sonnet", Label: "Sonnet", Description: "Balanced performance - best for most agents"},
	{Value: "opus", Label: "Opus", Description: "Most capable for complex reasoning tasks"},
	{Value: "haiku", Label: "Haiku", Description: "Fast and efficient for simple tasks"},
	{Value: "inherit", Label: "Inherit from parent", Description: "Use the same model as the main conversation"},
}

// ModelAlias 模型别名
type ModelAlias string

const (
	ModelSonnet ModelAlias = "sonnet"
	ModelOpus   ModelAlias = "opus"
	ModelHaiku  ModelAlias = "haiku"
	ModelBest   ModelAlias = "best"
)

// ValidModelAliases 有效的模型别名
var ValidModelAliases = []ModelAlias{ModelSonnet, ModelOpus, ModelHaiku, ModelBest}

// GetDefaultSubagentModel 获取默认子代理模型
func GetDefaultSubagentModel() string {
	return "inherit"
}

// GetAgentModel 计算代理的有效模型
// agentModel: 代理定义的模型设置
// parentModel: 父级模型的完整字符串
// toolSpecifiedModel: 工具调用时指定的模型（可选）
// permissionMode: 权限模式
func GetAgentModel(agentModel string, parentModel string, toolSpecifiedModel *string, permissionMode types.PermissionMode) string {
	// 环境变量覆盖
	if envModel := os.Getenv("CLAUDE_CODE_SUBAGENT_MODEL"); envModel != "" {
		return ParseUserSpecifiedModel(envModel)
	}

	// 处理工具指定的模型
	if toolSpecifiedModel != nil && *toolSpecifiedModel != "" {
		if aliasMatchesParentTier(*toolSpecifiedModel, parentModel) {
			return parentModel
		}
		return ParseUserSpecifiedModel(*toolSpecifiedModel)
	}

	// 使用代理定义的模型或默认值
	modelToUse := agentModel
	if modelToUse == "" {
		modelToUse = GetDefaultSubagentModel()
	}

	// 处理继承
	if modelToUse == "inherit" {
		// 返回父模型（实际应用中这里会有更复杂的运行时模型解析）
		return parentModel
	}

	// 检查是否匹配父级层级
	if aliasMatchesParentTier(modelToUse, parentModel) {
		return parentModel
	}

	// 解析并返回模型
	return ParseUserSpecifiedModel(modelToUse)
}

// aliasMatchesParentTier 检查简短的家族别名是否与父模型的层级匹配
// 当匹配时，子代理继承父级的确切模型字符串，而不是将别名解析为提供商默认值
func aliasMatchesParentTier(alias string, parentModel string) bool {
	canonical := strings.ToLower(parentModel)
	lowerAlias := strings.ToLower(alias)

	switch lowerAlias {
	case "opus":
		return strings.Contains(canonical, "opus")
	case "sonnet":
		return strings.Contains(canonical, "sonnet")
	case "haiku":
		return strings.Contains(canonical, "haiku")
	default:
		return false
	}
}

// ParseUserSpecifiedModel 解析用户指定的模型字符串
// 在实际实现中，这里会调用模型系统进行标准化
func ParseUserSpecifiedModel(model string) string {
	// 简单实现：返回规范化后的模型名称
	model = strings.TrimSpace(strings.ToLower(model))

	// 处理常见别名
	switch model {
	case "sonnet", "claude-3-sonnet", "claude-3-5-sonnet":
		return "claude-3-5-sonnet-20241022"
	case "opus", "claude-3-opus":
		return "claude-3-opus-20240229"
	case "haiku", "claude-3-haiku":
		return "claude-3-haiku-20240307"
	case "best":
		return "claude-3-opus-20240229"
	default:
		return model
	}
}

// GetAgentModelDisplay 获取代理模型的显示名称
func GetAgentModelDisplay(model string) string {
	if model == "" {
		return "Inherit from parent (default)"
	}
	if model == "inherit" {
		return "Inherit from parent"
	}

	// 映射到友好名称
	switch strings.ToLower(model) {
	case "sonnet", "claude-3-sonnet", "claude-3-5-sonnet", "claude-3-5-sonnet-20241022":
		return "Sonnet"
	case "opus", "claude-3-opus", "claude-3-opus-20240229":
		return "Opus"
	case "haiku", "claude-3-haiku", "claude-3-haiku-20240307":
		return "Haiku"
	default:
		return model
	}
}

// ValidateAgentModel 验证代理模型是否有效
func ValidateAgentModel(model string) bool {
	if model == "" || model == "inherit" {
		return true
	}

	// 检查是否为已知别名
	for _, alias := range ValidModelAliases {
		if ModelAlias(strings.ToLower(model)) == alias {
			return true
		}
	}

	// 检查是否为完整的模型ID（包含claude-3-或类似模式）
	lowerModel := strings.ToLower(model)
	return strings.Contains(lowerModel, "sonnet") ||
		strings.Contains(lowerModel, "opus") ||
		strings.Contains(lowerModel, "haiku")
}
