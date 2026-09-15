import { defineStore } from 'pinia'
import { ref } from 'vue'
import api from '../api/client'

export const useAlarmStore = defineStore('alarms', () => {
  const activeCount = ref(0)
  const perDevice = ref({})
  let timer = null

  async function refreshSummary() {
    try {
      const { data } = await api.get('/alarms/summary')
      activeCount.value = data.activeCount || 0
      perDevice.value = data.perDevice || {}
    } catch {
      // 角标刷新失败保持静默，不打断页面
    }
  }

  function startPolling(ms = 5000) {
    stopPolling()
    refreshSummary()
    timer = setInterval(refreshSummary, ms)
  }

  function stopPolling() {
    if (timer) {
      clearInterval(timer)
      timer = null
    }
  }

  return { activeCount, perDevice, refreshSummary, startPolling, stopPolling }
})
