<script setup>
import { ref, onMounted, nextTick } from 'vue'

const messages = ref([])
const inputText = ref('')
const isLoading = ref(false)
const messagesContainer = ref(null)

const addMessage = (content, isUser = false) => {
  messages.value.push({
    id: Date.now(),
    content,
    isUser,
    timestamp: new Date().toLocaleTimeString()
  })
  scrollToBottom()
}

const scrollToBottom = () => {
  nextTick(() => {
    if (messagesContainer.value) {
      messagesContainer.value.scrollTop = messagesContainer.value.scrollHeight
    }
  })
}

const sendMessage = async () => {
  if (!inputText.value.trim() || isLoading.value) return

  const userMessage = inputText.value.trim()
  inputText.value = ''
  
  addMessage(userMessage, true)
  isLoading.value = true

  // 模拟 AI 回复
  setTimeout(() => {
    addMessage('这是一个模拟的回复。在实际使用中，这里会连接到 DogClaw Gateway 来获取真实的 AI 回复。')
    isLoading.value = false
  }, 1000)
}

const handleKeyPress = (e) => {
  if (e.key === 'Enter' && !e.shiftKey) {
    e.preventDefault()
    sendMessage()
  }
}

onMounted(() => {
  addMessage('你好！我是 DogClaw AI 助手。有什么可以帮助你的吗？')
})
</script>

<template>
  <div class="chat">
    <div class="chat-container">
      <div class="messages" ref="messagesContainer">
        <div 
          v-for="message in messages" 
          :key="message.id"
          :class="['message', { 'user-message': message.isUser, 'ai-message': !message.isUser }]"
        >
          <div class="message-avatar">
            <span v-if="message.isUser">👤</span>
            <span v-else>🐾</span>
          </div>
          <div class="message-content">
            <div class="message-text">{{ message.content }}</div>
            <div class="message-time">{{ message.timestamp }}</div>
          </div>
        </div>
        <div v-if="isLoading" class="message ai-message">
          <div class="message-avatar">
            <span>🐾</span>
          </div>
          <div class="message-content">
            <div class="typing-indicator">
              <span></span>
              <span></span>
              <span></span>
            </div>
          </div>
        </div>
      </div>
      
      <div class="input-area">
        <textarea
          v-model="inputText"
          placeholder="输入你的消息..."
          @keypress="handleKeyPress"
          :disabled="isLoading"
          rows="1"
          class="message-input"
        />
        <button 
          @click="sendMessage"
          :disabled="isLoading || !inputText.trim()"
          class="send-btn"
        >
          发送
        </button>
      </div>
    </div>
  </div>
</template>

<style scoped>
.chat {
  height: 100%;
  display: flex;
  justify-content: center;
  align-items: center;
}

.chat-container {
  background: white;
  border-radius: 16px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
  width: 100%;
  max-width: 900px;
  height: calc(100vh - 120px);
  display: flex;
  flex-direction: column;
  overflow: hidden;
}

.messages {
  flex: 1;
  overflow-y: auto;
  padding: 24px;
  display: flex;
  flex-direction: column;
  gap: 16px;
}

.message {
  display: flex;
  gap: 12px;
  max-width: 80%;
}

.user-message {
  align-self: flex-end;
  flex-direction: row-reverse;
}

.ai-message {
  align-self: flex-start;
}

.message-avatar {
  width: 40px;
  height: 40px;
  border-radius: 50%;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  display: flex;
  align-items: center;
  justify-content: center;
  font-size: 20px;
  flex-shrink: 0;
}

.user-message .message-avatar {
  background: #f0f0f0;
}

.message-content {
  flex: 1;
}

.message-text {
  background: #f5f5f5;
  padding: 14px 18px;
  border-radius: 12px;
  line-height: 1.6;
  font-size: 15px;
}

.user-message .message-text {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
}

.message-time {
  font-size: 12px;
  color: #999;
  margin-top: 6px;
  padding: 0 4px;
}

.user-message .message-time {
  text-align: right;
}

.typing-indicator {
  background: #f5f5f5;
  padding: 18px;
  border-radius: 12px;
  display: flex;
  gap: 6px;
}

.typing-indicator span {
  width: 8px;
  height: 8px;
  background: #999;
  border-radius: 50%;
  animation: typing 1.4s infinite;
}

.typing-indicator span:nth-child(2) {
  animation-delay: 0.2s;
}

.typing-indicator span:nth-child(3) {
  animation-delay: 0.4s;
}

@keyframes typing {
  0%, 60%, 100% {
    transform: translateY(0);
    opacity: 0.4;
  }
  30% {
    transform: translateY(-8px);
    opacity: 1;
  }
}

.input-area {
  padding: 20px 24px 24px;
  border-top: 1px solid #eee;
  display: flex;
  gap: 12px;
  align-items: flex-end;
}

.message-input {
  flex: 1;
  padding: 14px 18px;
  border: 2px solid #e0e0e0;
  border-radius: 12px;
  font-size: 15px;
  font-family: inherit;
  resize: none;
  max-height: 120px;
  transition: border-color 0.3s;
}

.message-input:focus {
  outline: none;
  border-color: #667eea;
}

.message-input:disabled {
  background: #f5f5f5;
  cursor: not-allowed;
}

.send-btn {
  padding: 14px 28px;
  border: none;
  border-radius: 12px;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
  font-size: 15px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.3s;
}

.send-btn:hover:not(:disabled) {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(102, 126, 234, 0.4);
}

.send-btn:disabled {
  opacity: 0.5;
  cursor: not-allowed;
}
</style>
