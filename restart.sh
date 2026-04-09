#!/bin/bash

# --- 配置区 ---
APP_CMD="./dogclaw gateway LOG_LEVEL=debug"     # 程序 A 的启动命令
APP_NAME="dogclaw gateway LOG_LEVEL=debug"            # 用于匹配进程的关键字
LOG_FILE="logs/app.log"         # 日志文件
RESTART_SIGNAL=42          # 约定的重启信号

# --- 函数：后台守护运行 ---
do_daemon() {
    echo "守护进程已启动..."
    while true; do
        $APP_CMD >> $LOG_FILE 2>&1
        EXIT_CODE=$?

#        # 核心逻辑：如果退出码是 42，则重启
#        if [ $EXIT_CODE -eq $RESTART_SIGNAL ]; then
            echo "[$(date)] 收到重启信号 ($RESTART_SIGNAL)，正在重启..." >> $LOG_FILE
            sleep 1
#        else
#            echo "[$(date)] 程序正常退出或崩溃 (Code: $EXIT_CODE)，停止守护。" >> $LOG_FILE
#            break
#        fi
    done
}

# --- 指令处理 ---
case "$1" in
    start)
        # 检查是否已经在运行
        if pgrep -f "$APP_NAME" > /dev/null; then
            echo "错误: $APP_NAME 已经在运行中。"
        else
            echo "正在启动 $APP_NAME..."
            # 使用 nohup 在后台运行守护函数
            nohup bash "$0" daemon_internal >> $LOG_FILE 2>&1 &
            echo "启动成功，日志请查看 $LOG_FILE"
        fi
        ;;

    restart)
        PID=$(pgrep -f "$APP_NAME")
        if [ -z "$PID" ]; then
            echo "错误: 未发现正在运行的进程 $APP_NAME，尝试直接 start..."
            bash "$0" start
        else
            make
            echo "正在向进程 $PID 发送重启信号 ($RESTART_SIGNAL)..."
            kill  $PID
            echo "信号已发送。"
        fi
        ;;

    build)
        # 内部参数，不对外暴露，仅供 start 指令调用
        make
        ;;

    *)
        echo "用法: $0 {start|restart}"
        exit 1
        ;;
esac