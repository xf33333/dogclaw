package slash

import (
	"context"
	"fmt"
	"strings"

	"dogclaw/pkg/experience"
)

type ExperienceCommandHandler struct {
	manager *experience.Manager
}

func NewExperienceCommandHandler(manager *experience.Manager) *ExperienceCommandHandler {
	return &ExperienceCommandHandler{manager: manager}
}

func HandleExperience(ctx context.Context, args string, manager *experience.Manager) (*CommandResult, error) {
	if manager == nil {
		return &CommandResult{
			IsError:  true,
			ErrorMsg: "Experience manager is not initialized",
		}, nil
	}

	parts := strings.Fields(args)
	if len(parts) == 0 {
		return handleExperienceList(manager)
	}

	action := parts[0]
	switch action {
	case "list", "ls":
		return handleExperienceList(manager)
	case "read", "r":
		if len(parts) < 2 {
			return &CommandResult{
				IsError:  true,
				ErrorMsg: "Usage: /experience read <date>",
			}, nil
		}
		return handleExperienceRead(manager, parts[1])
	case "summary", "s":
		if len(parts) < 2 {
			return &CommandResult{
				IsError:  true,
				ErrorMsg: "Usage: /experience summary <date>",
			}, nil
		}
		return handleExperienceSummary(ctx, manager, parts[1])
	case "regenerate", "regen":
		if len(parts) < 2 {
			return &CommandResult{
				IsError:  true,
				ErrorMsg: "Usage: /experience regenerate <date>",
			}, nil
		}
		return handleExperienceRegenerate(ctx, manager, parts[1])
	default:
		return &CommandResult{
			IsError:  true,
			ErrorMsg: fmt.Sprintf("Unknown action: %s. Supported actions: list, read, summary, regenerate", action),
		}, nil
	}
}

func handleExperienceList(manager *experience.Manager) (*CommandResult, error) {
	files, err := manager.GetExperienceList()
	if err != nil {
		return &CommandResult{
			IsError:  true,
			ErrorMsg: fmt.Sprintf("Failed to get experience list: %v", err),
		}, nil
	}

	if len(files) == 0 {
		return &CommandResult{
			Output: "暂无经验记录。经验会在每天自动总结生成。",
		}, nil
	}

	var result strings.Builder
	result.WriteString(fmt.Sprintf("共找到 %d 条经验记录：\n\n", len(files)))

	for i, f := range files {
		sizeKB := float64(f.Size) / 1024
		result.WriteString(fmt.Sprintf("%d. %s - %.1f KB (修改时间: %s)\n",
			i+1, f.Date, sizeKB, f.ModTime.Format("2006-01-02 15:04")))
	}

	result.WriteString("\n使用 /experience read <date> 来查看具体某天的经验。")

	return &CommandResult{
		Output: result.String(),
	}, nil
}

func handleExperienceRead(manager *experience.Manager, date string) (*CommandResult, error) {
	content, err := manager.GetExperience(date)
	if err != nil {
		return &CommandResult{
			IsError:  true,
			ErrorMsg: fmt.Sprintf("Failed to read experience for %s: %v", date, err),
		}, nil
	}

	return &CommandResult{
		Output: content,
	}, nil
}

func handleExperienceSummary(ctx context.Context, manager *experience.Manager, date string) (*CommandResult, error) {
	if manager.HasExperience(date) {
		return &CommandResult{
			IsError:  true,
			ErrorMsg: fmt.Sprintf("经验文件 %s 已存在，如需重新生成，请使用 /experience regenerate %s", date, date),
		}, nil
	}

	if err := manager.ManualTriggerSummary(ctx, date); err != nil {
		return &CommandResult{
			IsError:  true,
			ErrorMsg: fmt.Sprintf("生成 %s 的经验总结失败: %v", date, err),
		}, nil
	}

	return &CommandResult{
		Output: fmt.Sprintf("✅ 成功生成 %s 的经验总结", date),
	}, nil
}

func handleExperienceRegenerate(ctx context.Context, manager *experience.Manager, date string) (*CommandResult, error) {
	if err := manager.ForceRegenerateSummary(ctx, date); err != nil {
		return &CommandResult{
			IsError:  true,
			ErrorMsg: fmt.Sprintf("重新生成 %s 的经验总结失败: %v", date, err),
		}, nil
	}

	return &CommandResult{
		Output: fmt.Sprintf("✅ 成功重新生成 %s 的经验总结", date),
	}, nil
}
