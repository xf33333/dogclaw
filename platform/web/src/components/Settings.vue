<script setup>
import { ref, onMounted } from 'vue'

const settings = ref({
  activeAlias: 'default',
  providers: [
    {
      alias: 'default',
      provider: 'openai',
      model: 'gpt-4',
      url: 'https://api.openai.com/v1',
      apiKey: ''
    }
  ],
  channel: {
    qq: {
      enabled: false,
      appId: '',
      appSecret: '',
      allowFrom: '',
      sendMarkdown: true
    },
    weixin: {
      enabled: false,
      token: ''
    },
    gateway: {
      enabled: true,
      port: 10086
    }
  },
  enableHeartbeat: true,
  heartbeatPeriod: 30,
  heartbeatTimeout: 60,
  maxTurns: 100,
  maxTokens: 4096,
  maxContextLength: 8192,
  maxBudgetUSD: 10.0,
  permissionMode: 'ask',
  verbose: false,
  temperature: 0.7,
  topP: 0.9,
  thinkingBudget: 1024,
  showToolUsageInReply: true,
  showThinkingInLog: false
})

const activeTab = ref('general')
const showSuccess = ref(false)
const showError = ref(false)
const errorMessage = ref('')

const saveSettings = () => {
  try {
    // 验证设置
    if (!settings.value.activeAlias) {
      throw new Error('activeAlias 不能为空')
    }
    if (settings.value.providers.length === 0) {
      throw new Error('providers 列表不能为空')
    }
    
    // 检查 activeAlias 是否存在
    const found = settings.value.providers.some(p => p.alias === settings.value.activeAlias)
    if (!found) {
      throw new Error(`activeAlias "${settings.value.activeAlias}" 不在 providers 列表中`)
    }

    // 模拟保存
    showSuccessMessage()
  } catch (error) {
    showErrorMessage(error.message)
  }
}

const showSuccessMessage = () => {
  showSuccess.value = true
  setTimeout(() => {
    showSuccess.value = false
  }, 3000)
}

const showErrorMessage = (msg) => {
  errorMessage.value = msg
  showError.value = true
  setTimeout(() => {
    showError.value = false
  }, 5000)
}

const formatJSON = () => {
  try {
    // 模拟格式化
    showSuccessMessage()
  } catch (error) {
    showErrorMessage('JSON 格式化失败: ' + error.message)
  }
}

const validateJSON = () => {
  try {
    // 模拟验证
    showSuccessMessage()
  } catch (error) {
    showErrorMessage('JSON 验证失败: ' + error.message)
  }
}

const addProvider = () => {
  settings.value.providers.push({
    alias: `provider-${settings.value.providers.length + 1}`,
    provider: 'openai',
    model: 'gpt-4',
    url: 'https://api.openai.com/v1',
    apiKey: ''
  })
}

const removeProvider = (index) => {
  if (settings.value.providers.length > 1) {
    settings.value.providers.splice(index, 1)
  }
}
</script>

<template>
  <div class="settings">
    <div class="settings-container">
      <div class="settings-header">
        <h1>设置</h1>
        <div class="header-actions">
          <button @click="formatJSON" class="btn-secondary">格式化</button>
          <button @click="validateJSON" class="btn-secondary">验证</button>
          <button @click="saveSettings" class="btn-primary">保存</button>
        </div>
      </div>

      <div v-if="showSuccess" class="message success">
        ✓ 操作成功！
      </div>
      <div v-if="showError" class="message error">
        ✗ {{ errorMessage }}
      </div>

      <div class="settings-tabs">
        <button 
          :class="['tab-btn', { active: activeTab === 'general' }]"
          @click="activeTab = 'general'"
        >
          通用设置
        </button>
        <button 
          :class="['tab-btn', { active: activeTab === 'providers' }]"
          @click="activeTab = 'providers'"
        >
          提供商
        </button>
        <button 
          :class="['tab-btn', { active: activeTab === 'channels' }]"
          @click="activeTab = 'channels'"
        >
          渠道
        </button>
        <button 
          :class="['tab-btn', { active: activeTab === 'advanced' }]"
          @click="activeTab = 'advanced'"
        >
          高级
        </button>
      </div>

      <div class="settings-content">
        <!-- 通用设置 -->
        <div v-if="activeTab === 'general'" class="settings-section">
          <h2>通用设置</h2>
          
          <div class="form-group">
            <label>活跃别名</label>
            <select v-model="settings.activeAlias" class="form-select">
              <option v-for="provider in settings.providers" :key="provider.alias" :value="provider.alias">
                {{ provider.alias }}
              </option>
            </select>
          </div>

          <div class="form-group">
            <label>温度</label>
            <input type="number" v-model.number="settings.temperature" step="0.1" min="0" max="2" class="form-input" />
          </div>

          <div class="form-group">
            <label>Top P</label>
            <input type="number" v-model.number="settings.topP" step="0.1" min="0" max="1" class="form-input" />
          </div>

          <div class="form-group checkbox">
            <input type="checkbox" v-model="settings.verbose" id="verbose" />
            <label for="verbose">详细日志</label>
          </div>
        </div>

        <!-- 提供商设置 -->
        <div v-if="activeTab === 'providers'" class="settings-section">
          <div class="section-header">
            <h2>提供商配置</h2>
            <button @click="addProvider" class="btn-primary btn-small">+ 添加提供商</button>
          </div>

          <div v-for="(provider, index) in settings.providers" :key="index" class="provider-card">
            <div class="provider-header">
              <h3>{{ provider.alias }}</h3>
              <button 
                v-if="settings.providers.length > 1"
                @click="removeProvider(index)" 
                class="btn-danger btn-small"
              >
                删除
              </button>
            </div>

            <div class="form-grid">
              <div class="form-group">
                <label>别名</label>
                <input type="text" v-model="provider.alias" class="form-input" />
              </div>

              <div class="form-group">
                <label>提供商</label>
                <select v-model="provider.provider" class="form-select">
                  <option value="openai">OpenAI</option>
                  <option value="anthropic">Anthropic</option>
                  <option value="ollama">Ollama</option>
                </select>
              </div>

              <div class="form-group">
                <label>模型</label>
                <input type="text" v-model="provider.model" class="form-input" />
              </div>

              <div class="form-group">
                <label>API URL</label>
                <input type="text" v-model="provider.url" class="form-input" />
              </div>

              <div class="form-group full-width">
                <label>API Key</label>
                <input type="password" v-model="provider.apiKey" class="form-input" />
              </div>
            </div>
          </div>
        </div>

        <!-- 渠道设置 -->
        <div v-if="activeTab === 'channels'" class="settings-section">
          <h2>渠道配置</h2>

          <div class="channel-card">
            <div class="form-group checkbox">
              <input type="checkbox" v-model="settings.channel.gateway.enabled" id="gateway-enabled" />
              <label for="gateway-enabled">启用 Gateway</label>
            </div>
            <div class="form-group">
              <label>端口</label>
              <input type="number" v-model.number="settings.channel.gateway.port" class="form-input" />
            </div>
          </div>

          <div class="channel-card">
            <div class="form-group checkbox">
              <input type="checkbox" v-model="settings.channel.qq.enabled" id="qq-enabled" />
              <label for="qq-enabled">启用 QQ</label>
            </div>
            <div class="form-grid" v-if="settings.channel.qq.enabled">
              <div class="form-group">
                <label>App ID</label>
                <input type="text" v-model="settings.channel.qq.appId" class="form-input" />
              </div>
              <div class="form-group">
                <label>App Secret</label>
                <input type="password" v-model="settings.channel.qq.appSecret" class="form-input" />
              </div>
              <div class="form-group">
                <label>允许来源</label>
                <input type="text" v-model="settings.channel.qq.allowFrom" class="form-input" />
              </div>
              <div class="form-group checkbox">
                <input type="checkbox" v-model="settings.channel.qq.sendMarkdown" id="qq-markdown" />
                <label for="qq-markdown">发送 Markdown</label>
              </div>
            </div>
          </div>

          <div class="channel-card">
            <div class="form-group checkbox">
              <input type="checkbox" v-model="settings.channel.weixin.enabled" id="weixin-enabled" />
              <label for="weixin-enabled">启用微信</label>
            </div>
            <div class="form-group" v-if="settings.channel.weixin.enabled">
              <label>Token</label>
              <input type="text" v-model="settings.channel.weixin.token" class="form-input" />
            </div>
          </div>
        </div>

        <!-- 高级设置 -->
        <div v-if="activeTab === 'advanced'" class="settings-section">
          <h2>高级设置</h2>

          <div class="form-grid">
            <div class="form-group">
              <label>最大对话轮数</label>
              <input type="number" v-model.number="settings.maxTurns" class="form-input" />
            </div>

            <div class="form-group">
              <label>最大 Token</label>
              <input type="number" v-model.number="settings.maxTokens" class="form-input" />
            </div>

            <div class="form-group">
              <label>最大上下文长度</label>
              <input type="number" v-model.number="settings.maxContextLength" class="form-input" />
            </div>

            <div class="form-group">
              <label>最大预算 (USD)</label>
              <input type="number" v-model.number="settings.maxBudgetUSD" step="0.1" class="form-input" />
            </div>

            <div class="form-group">
              <label>心跳周期 (秒)</label>
              <input type="number" v-model.number="settings.heartbeatPeriod" class="form-input" />
            </div>

            <div class="form-group">
              <label>心跳超时 (秒)</label>
              <input type="number" v-model.number="settings.heartbeatTimeout" class="form-input" />
            </div>

            <div class="form-group">
              <label>思考预算</label>
              <input type="number" v-model.number="settings.thinkingBudget" class="form-input" />
            </div>

            <div class="form-group">
              <label>权限模式</label>
              <select v-model="settings.permissionMode" class="form-select">
                <option value="ask">询问</option>
                <option value="auto">自动</option>
                <option value="deny">拒绝</option>
              </select>
            </div>
          </div>

          <div class="form-group checkbox">
            <input type="checkbox" v-model="settings.enableHeartbeat" id="enable-heartbeat" />
            <label for="enable-heartbeat">启用心跳</label>
          </div>

          <div class="form-group checkbox">
            <input type="checkbox" v-model="settings.showToolUsageInReply" id="show-tool-usage" />
            <label for="show-tool-usage">在回复中显示工具使用</label>
          </div>

          <div class="form-group checkbox">
            <input type="checkbox" v-model="settings.showThinkingInLog" id="show-thinking" />
            <label for="show-thinking">在日志中显示思考过程</label>
          </div>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.settings {
  height: 100%;
  display: flex;
  justify-content: center;
  align-items: flex-start;
  padding-top: 20px;
}

.settings-container {
  background: white;
  border-radius: 16px;
  box-shadow: 0 20px 60px rgba(0, 0, 0, 0.3);
  width: 100%;
  max-width: 1000px;
  max-height: calc(100vh - 140px);
  overflow-y: auto;
}

.settings-header {
  padding: 24px 30px;
  border-bottom: 1px solid #eee;
  display: flex;
  justify-content: space-between;
  align-items: center;
}

.settings-header h1 {
  font-size: 24px;
  font-weight: 700;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.header-actions {
  display: flex;
  gap: 10px;
}

.message {
  margin: 0 30px 20px;
  padding: 14px 18px;
  border-radius: 8px;
  font-size: 14px;
}

.message.success {
  background: #e8f5e9;
  color: #2e7d32;
  border: 1px solid #a5d6a7;
}

.message.error {
  background: #ffebee;
  color: #c62828;
  border: 1px solid #ef9a9a;
}

.settings-tabs {
  padding: 0 30px;
  display: flex;
  gap: 8px;
  border-bottom: 1px solid #eee;
}

.tab-btn {
  padding: 14px 20px;
  border: none;
  background: none;
  font-size: 14px;
  font-weight: 500;
  color: #666;
  cursor: pointer;
  border-bottom: 2px solid transparent;
  margin-bottom: -1px;
  transition: all 0.3s;
}

.tab-btn:hover {
  color: #667eea;
}

.tab-btn.active {
  color: #667eea;
  border-bottom-color: #667eea;
}

.settings-content {
  padding: 30px;
}

.settings-section h2 {
  font-size: 18px;
  font-weight: 600;
  margin-bottom: 24px;
  color: #333;
}

.section-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 24px;
}

.section-header h2 {
  margin-bottom: 0;
}

.form-grid {
  display: grid;
  grid-template-columns: repeat(2, 1fr);
  gap: 20px;
}

.form-group {
  margin-bottom: 20px;
}

.form-group.full-width {
  grid-column: 1 / -1;
}

.form-group label {
  display: block;
  font-size: 14px;
  font-weight: 500;
  margin-bottom: 8px;
  color: #555;
}

.form-input,
.form-select {
  width: 100%;
  padding: 12px 14px;
  border: 2px solid #e0e0e0;
  border-radius: 8px;
  font-size: 14px;
  font-family: inherit;
  transition: border-color 0.3s;
}

.form-input:focus,
.form-select:focus {
  outline: none;
  border-color: #667eea;
}

.form-group.checkbox {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 16px;
}

.form-group.checkbox input[type="checkbox"] {
  width: 18px;
  height: 18px;
  cursor: pointer;
}

.form-group.checkbox label {
  margin-bottom: 0;
  cursor: pointer;
}

.provider-card,
.channel-card {
  background: #f8f9fa;
  border-radius: 12px;
  padding: 20px;
  margin-bottom: 20px;
}

.provider-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  margin-bottom: 20px;
}

.provider-header h3 {
  font-size: 16px;
  font-weight: 600;
  color: #333;
}

.btn-primary,
.btn-secondary,
.btn-danger {
  padding: 10px 20px;
  border: none;
  border-radius: 8px;
  font-size: 14px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.3s;
}

.btn-primary {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
}

.btn-primary:hover {
  transform: translateY(-2px);
  box-shadow: 0 4px 12px rgba(102, 126, 234, 0.4);
}

.btn-secondary {
  background: #f0f0f0;
  color: #333;
}

.btn-secondary:hover {
  background: #e0e0e0;
}

.btn-danger {
  background: #ff4757;
  color: white;
}

.btn-danger:hover {
  background: #ff3838;
}

.btn-small {
  padding: 6px 14px;
  font-size: 13px;
}
</style>
