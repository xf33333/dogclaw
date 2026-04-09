#!/bin/bash

# DogClaw 重启脚本
# 用于保存运行参数，重新编译后用同样参数重启

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
PID_FILE="$SCRIPT_DIR/.dogclaw.pid"
ARGS_FILE="$SCRIPT_DIR/.dogclaw.args"
ENV_FILE="$SCRIPT_DIR/.dogclaw.env"
BINARY_NAME="dogclaw"
BINARY_PATH="$SCRIPT_DIR/$BINARY_NAME"

# 颜色输出
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

log_info() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

log_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

log_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

log_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

# 保存运行参数和环境变量
save_state() {
    local pid=$1
    shift
    log_info "保存运行参数..."
    
    # 保存PID
    echo "$pid" > "$PID_FILE"
    
    # 保存命令行参数
    echo "$*" > "$ARGS_FILE"
    
    # 保存相关环境变量
    {
        echo "ANTHROPIC_API_KEY=$ANTHROPIC_API_KEY"
        echo "OPENROUTER_API_KEY=$OPENROUTER_API_KEY"
    } > "$ENV_FILE"
    
    log_success "状态已保存"
    log_info "  PID: $pid"
    log_info "  Args: $*"
}

# 加载保存的状态
load_state() {
    if [[ ! -f "$ARGS_FILE" ]]; then
        log_error "没有找到保存的参数文件"
        return 1
    fi
    
    # 加载参数
    ARGS=$(cat "$ARGS_FILE")
    
    # 加载环境变量
    if [[ -f "$ENV_FILE" ]]; then
        while IFS= read -r line; do
            export "$line"
        done < "$ENV_FILE"
    fi
    
    log_info "已加载保存的状态"
    log_info "  Args: $ARGS"
}

# 停止运行中的进程
stop_process() {
    if [[ -f "$PID_FILE" ]]; then
        local pid=$(cat "$PID_FILE" 2>/dev/null)
        if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
            log_info "正在停止进程 $pid..."
            kill "$pid"
            # 等待进程结束
            for i in {1..10}; do
                if ! kill -0 "$pid" 2>/dev/null; then
                    break
                fi
                sleep 0.5
            done
            # 如果还在运行，强制杀死
            if kill -0 "$pid" 2>/dev/null; then
                log_warning "进程没有响应，强制终止..."
                kill -9 "$pid"
            fi
            log_success "进程已停止"
        fi
        rm -f "$PID_FILE"
    fi
}

# 重新编译
rebuild() {
    log_info "开始重新编译..."
    cd "$SCRIPT_DIR" || exit 1
    
    if make build; then
        log_success "编译成功"
        return 0
    else
        log_error "编译失败"
        return 1
    fi
}

# 启动进程
start_process() {
    local args="$1"
    log_info "使用参数启动: $args"
    
    cd "$SCRIPT_DIR" || exit 1
    
    if [[ ! -x "$BINARY_PATH" ]]; then
        log_error "二进制文件不存在: $BINARY_PATH"
        return 1
    fi
    
    # 在后台启动，保存PID
    nohup "$BINARY_PATH" $args > /dev/null 2>&1 &
    local pid=$!
    
    # 保存新的状态
    save_state "$pid" $args
    
    log_success "进程已启动，PID: $pid"
    
    # 等待一下确保进程正常启动
    sleep 1
    if ! kill -0 "$pid" 2>/dev/null; then
        log_error "进程启动失败，请检查日志"
        return 1
    fi
    
    return 0
}

# 显示帮助
show_help() {
    echo "DogClaw 重启脚本"
    echo ""
    echo "用法: $0 [命令 [参数]]"
    echo ""
    echo "命令:"
    echo "  start <mode>  - 启动并保存状态 (例如: $0 start agent)"
    echo "  restart        - 重新编译并重启 (使用保存的参数)"
    echo "  stop           - 停止运行中的进程"
    echo "  status         - 查看状态"
    echo "  help           - 显示帮助"
    echo ""
    echo "示例:"
    echo "  # 首次启动"
    echo "    $0 start agent"
    echo ""
    echo "  # 修改代码后重启"
    echo "    $0 restart"
}

# 查看状态
show_status() {
    echo "DogClaw 状态"
    echo "=============="
    
    if [[ -f "$PID_FILE" ]]; then
        local pid=$(cat "$PID_FILE" 2>/dev/null)
        if [[ -n "$pid" ]] && kill -0 "$pid" 2>/dev/null; then
            log_success "进程运行中: PID=$pid"
        else
            log_warning "PID文件存在但进程未运行"
        fi
    else
        log_info "没有运行中的进程"
    fi
    
    if [[ -f "$ARGS_FILE" ]]; then
        log_info "保存的参数: $(cat "$ARGS_FILE")"
    fi
}

# 主函数
main() {
    local cmd="$1"
    shift
    
    case "$cmd" in
        start)
            if [[ $# -eq 0 ]]; then
                log_error "请指定模式 (agent/gateway/onboard)"
                exit 1
            fi
            stop_process
            if rebuild; then
                start_process "$*"
            fi
            ;;
        restart)
            load_state || exit 1
            stop_process
            if rebuild; then
                start_process "$ARGS"
            fi
            ;;
        stop)
            stop_process
            ;;
        status)
            show_status
            ;;
        help|--help|-h)
            show_help
            ;;
        *)
            log_error "未知命令: $cmd"
            echo ""
            show_help
            exit 1
            ;;
    esac
}

# 执行主函数
main "$@"