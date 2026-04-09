
#!/bin/bash

# DogClaw 重启脚本
# 用于编译、启动和重启 DogClaw 服务

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR" || exit 1

BINARY_NAME="dogclaw"
APP_CMD="./$BINARY_NAME gateway"
APP_NAME="$BINARY_NAME gateway"
LOG_DIR="logs"
LOG_FILE="$LOG_DIR/app.log"
RESTART_SIGNAL=42

mkdir -p "$LOG_DIR"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

# 编译程序
do_build() {
    log "正在编译 DogClaw..."
    if make build; then
        log "编译成功"
        return 0
    else
        log "编译失败"
        return 1
    fi
}

# 启动程序（前台运行）
do_start() {
    log "启动 DogClaw..."
    $APP_CMD >> "$LOG_FILE" 2>&1
    return $?
}

# 检查进程是否在运行
is_running() {
    pgrep -f "$APP_NAME" > /dev/null 2>&1
}

# 获取进程 PID
get_pid() {
    pgrep -f "$APP_NAME"
}

case "$1" in
    build)
        do_build
        exit $?
        ;;

    start)
        if is_running; then
            echo "错误: DogClaw 已经在运行中 (PID: $(get_pid))"
            exit 1
        fi
        
        if [ ! -f "./$BINARY_NAME" ]; then
            echo "二进制文件不存在，先编译..."
            if ! do_build; then
                exit 1
            fi
        fi
        
        do_start
        exit $?
        ;;

    restart)
        if ! is_running; then
            echo "DogClaw 未运行，尝试直接启动..."
            bash "$0" start
            exit $?
        fi
        
        PID=$(get_pid)
        echo "正在重新编译 DogClaw..."
        if ! do_build; then
            echo "编译失败，保持当前进程运行"
            exit 1
        fi
        
        echo "正在向进程 $PID 发送重启信号 ($RESTART_SIGNAL)..."
        kill -$RESTART_SIGNAL $PID
        echo "信号已发送，服务将自动重启"
        log "已发送重启信号给进程 $PID"
        ;;

    stop)
        if ! is_running; then
            echo "DogClaw 未运行"
            exit 0
        fi
        
        PID=$(get_pid)
        echo "正在停止 DogClaw (PID: $PID)..."
        kill $PID
        log "正在停止 DogClaw (PID: $PID)"
        ;;

    status)
        if is_running; then
            echo "DogClaw 正在运行 (PID: $(get_pid))"
        else
            echo "DogClaw 未运行"
        fi
        ;;

    *)
        echo "用法: $0 {build|start|restart|stop|status}"
        echo ""
        echo "  build   - 编译 DogClaw"
        echo "  start   - 启动 DogClaw（前台运行）"
        echo "  restart - 重新编译并重启 DogClaw"
        echo "  stop    - 停止 DogClaw"
        echo "  status  - 查看 DogClaw 运行状态"
        exit 1
        ;;
esac

