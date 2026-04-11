<script setup>
import { ref } from 'vue'
import Chat from './components/Chat.vue'
import Settings from './components/Settings.vue'

const activeTab = ref('chat')
</script>

<template>
  <div class="app">
    <header class="header">
      <div class="logo">
        <span class="logo-icon">🐾</span>
        <span class="logo-text">DogClaw</span>
      </div>
      <nav class="nav">
        <button 
          :class="['nav-btn', { active: activeTab === 'chat' }]"
          @click="activeTab = 'chat'"
        >
          聊天
        </button>
        <button 
          :class="['nav-btn', { active: activeTab === 'settings' }]"
          @click="activeTab = 'settings'"
        >
          设置
        </button>
      </nav>
    </header>
    
    <main class="main">
      <Chat v-if="activeTab === 'chat'" />
      <Settings v-if="activeTab === 'settings'" />
    </main>
  </div>
</template>

<style scoped>
.app {
  min-height: 100vh;
  display: flex;
  flex-direction: column;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
}

.header {
  background: rgba(255, 255, 255, 0.95);
  padding: 20px 30px;
  display: flex;
  justify-content: space-between;
  align-items: center;
  box-shadow: 0 2px 20px rgba(0, 0, 0, 0.1);
}

.logo {
  display: flex;
  align-items: center;
  gap: 12px;
}

.logo-icon {
  font-size: 32px;
}

.logo-text {
  font-size: 24px;
  font-weight: 700;
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  -webkit-background-clip: text;
  -webkit-text-fill-color: transparent;
  background-clip: text;
}

.nav {
  display: flex;
  gap: 12px;
}

.nav-btn {
  padding: 10px 24px;
  border: none;
  border-radius: 8px;
  font-size: 15px;
  font-weight: 500;
  cursor: pointer;
  transition: all 0.3s;
  background: #f0f0f0;
  color: #333;
}

.nav-btn:hover {
  background: #e0e0e0;
}

.nav-btn.active {
  background: linear-gradient(135deg, #667eea 0%, #764ba2 100%);
  color: white;
}

.main {
  flex: 1;
  padding: 20px;
}
</style>
