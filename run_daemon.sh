
#!/bin/bash

# DogClaw 守护进程脚本
# 监听程序退出，若退出码为42则自动重新启动

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR" || exit 1

RESTART_SCRIPT="./restart.sh"
LOG_DIR="logs"
LOG_FILE="$LOG_DIR/daemon.log"

mkdir -p "$LOG_DIR"

log() {
    echo "[$(date '+%Y-%m-%d %H:%M:%S')] $1" | tee -a "$LOG_FILE"
}

trap 'log "收到终止信号，正在停止..."; exit 0' SIGINT SIGTERM

log "守护进程启动"

while true; do
    log "启动 DogClaw..."
    
    # 使用 restart.sh start 启动
    "$RESTART_SCRIPT" start "$@"
    
    # 获取退出码
    EXIT_CODE=$?
    
    log "DogClaw 已停止，退出码: $EXIT_CODE"
    
    # 检查是否因为信号12退出
    if [ "$EXIT_CODE" -eq 12 ]; then
        log "检测到重启信号（退出码12），准备重新启动..."
        sleep 1
    else
        log "非重启退出，守护进程停止"
        exit "$EXIT_CODE"
    fi
done

