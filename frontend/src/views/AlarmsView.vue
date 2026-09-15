<template>
  <div class="card-panel">
    <div style="display:flex;justify-content:space-between;align-items:center;margin-bottom:14px;gap:12px;flex-wrap:wrap">
      <div>
        <h2 style="margin:0 0 4px;font-size:18px">
          报警中心
          <el-badge v-if="activeTotal" :value="activeTotal" type="danger" style="margin-left:10px" />
        </h2>
        <div class="sub" style="margin:0">snapshot 读值超过高/低阈值时生成事件；恢复正常后自动消除</div>
      </div>
      <el-button :loading="loadingEvents" @click="loadEvents">刷新</el-button>
    </div>

    <el-tabs v-model="tab">
      <!-- 报警事件 -->
      <el-tab-pane name="events">
        <template #label>
          报警事件<el-badge v-if="activeTotal" :value="activeTotal" type="danger" class="tab-badge" />
        </template>

        <div style="display:flex;gap:10px;margin-bottom:12px;flex-wrap:wrap">
          <el-radio-group v-model="statusFilter" size="small" @change="loadEvents">
            <el-radio-button label="active">活跃</el-radio-button>
            <el-radio-button label="resolved">历史</el-radio-button>
            <el-radio-button label="all">全部</el-radio-button>
          </el-radio-group>
          <el-select v-model="deviceFilter" placeholder="全部设备" clearable size="small" style="width:200px" @change="loadEvents">
            <el-option v-for="d in devices" :key="d.id" :label="`${d.id} · ${d.name}`" :value="d.id" />
          </el-select>
        </div>

        <el-table :data="events" v-loading="loadingEvents" empty-text="暂无报警">
          <el-table-column label="级别" width="80">
            <template #default="{ row }">
              <el-tag :type="row.level === 'high' ? 'danger' : 'warning'" size="small">
                {{ row.level === 'high' ? '高报' : '低报' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column label="状态" width="90">
            <template #default="{ row }">
              <el-tag :type="row.status === 'active' ? 'danger' : 'info'" size="small" effect="plain">
                {{ row.status === 'active' ? '活跃' : '已消除' }}
              </el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="deviceId" label="设备" min-width="130" />
          <el-table-column prop="point" label="点位" min-width="120" />
          <el-table-column label="阈值" width="100">
            <template #default="{ row }"><span class="mono">{{ fmt(row.threshold) }}</span></template>
          </el-table-column>
          <el-table-column label="触发值" width="100">
            <template #default="{ row }"><span class="mono">{{ fmt(row.triggerValue) }}</span></template>
          </el-table-column>
          <el-table-column label="最新值" width="100">
            <template #default="{ row }"><span class="mono">{{ fmt(row.lastValue) }}</span></template>
          </el-table-column>
          <el-table-column prop="triggeredAt" label="触发时间" min-width="170" />
          <el-table-column prop="resolvedAt" label="消除时间" min-width="170">
            <template #default="{ row }">{{ row.resolvedAt || '—' }}</template>
          </el-table-column>
          <el-table-column label="操作" width="90" fixed="right">
            <template #default="{ row }">
              <el-button
                v-if="row.status === 'active'"
                type="primary"
                link
                size="small"
                :disabled="!auth.canWrite"
                @click="clearOne(row)"
              >消除</el-button>
              <span v-else class="sub">—</span>
            </template>
          </el-table-column>
        </el-table>
      </el-tab-pane>

      <!-- 报警规则 -->
      <el-tab-pane label="规则维护" name="rules">
        <template #label>
          <span>规则维护</span>
        </template>
        <div style="display:flex;justify-content:flex-end;margin-bottom:12px">
          <el-button type="primary" size="small" :disabled="!auth.canWrite" @click="openRule()">新增规则</el-button>
        </div>
        <el-table :data="rules" v-loading="loadingRules" empty-text="暂无规则（规则仅校验读值，不影响写值 min/max）">
          <el-table-column prop="deviceId" label="设备" min-width="130" />
          <el-table-column prop="point" label="点位" min-width="120" />
          <el-table-column label="高阈值" width="110">
            <template #default="{ row }"><span class="mono">{{ row.high == null ? '—' : fmt(row.high) }}</span></template>
          </el-table-column>
          <el-table-column label="低阈值" width="110">
            <template #default="{ row }"><span class="mono">{{ row.low == null ? '—' : fmt(row.low) }}</span></template>
          </el-table-column>
          <el-table-column label="启用" width="80">
            <template #default="{ row }">
              <el-tag :type="row.enabled ? 'success' : 'info'" size="small">{{ row.enabled ? '是' : '否' }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column label="操作" width="130" fixed="right">
            <template #default="{ row }">
              <el-button type="primary" link size="small" :disabled="!auth.canWrite" @click="openRule(row)">编辑</el-button>
              <el-popconfirm title="删除规则？活跃报警会自动消除" @confirm="removeRule(row)">
                <template #reference>
                  <el-button type="danger" link size="small" :disabled="!auth.canWrite">删除</el-button>
                </template>
              </el-popconfirm>
            </template>
          </el-table-column>
        </el-table>
        <p class="sub" style="margin-top:10px">
          说明：报警阈值面向 snapshot 读值越界；映射中 min/max 仍是写值时的合法范围校验，二者互不影响。observer 仅可查看。
        </p>
      </el-tab-pane>
    </el-tabs>

    <!-- 规则编辑弹窗 -->
    <el-dialog v-model="ruleVisible" :title="editing.deviceId ? '编辑报警规则' : '新增报警规则'" width="460px" destroy-on-close>
      <el-form label-position="top">
        <el-form-item label="设备">
          <el-select v-model="editing.deviceId" :disabled="!!editing._origin || !auth.canWrite" style="width:100%" @change="onDeviceChange">
            <el-option v-for="d in devices" :key="d.id" :label="`${d.id} · ${d.name}`" :value="d.id" />
          </el-select>
        </el-form-item>
        <el-form-item label="点位">
          <el-select v-model="editing.point" :disabled="!!editing._origin || !auth.canWrite || !editing.deviceId" style="width:100%">
            <el-option v-for="p in pointsOf(editing.deviceId)" :key="p.name" :label="`${p.name} @${p.address}`" :value="p.name" />
          </el-select>
        </el-form-item>
        <el-form-item label="高阈值（读值 &gt; 阈值时高报）">
          <el-input-number v-model="editing.high" :controls="false" style="width:100%" placeholder="不填表示不启用" />
        </el-form-item>
        <el-form-item label="低阈值（读值 &lt; 阈值时低报）">
          <el-input-number v-model="editing.low" :controls="false" style="width:100%" placeholder="不填表示不启用" />
        </el-form-item>
        <el-form-item>
          <el-switch v-model="editing.enabled" active-text="启用" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="ruleVisible = false">取消</el-button>
        <el-button type="primary" :loading="savingRule" @click="saveRule">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage } from 'element-plus'
import api from '../api/client'
import { useAuthStore } from '../stores/auth'
import { useAlarmStore } from '../stores/alarm'

const auth = useAuthStore()
const alarmStore = useAlarmStore()

const tab = ref('events')
const events = ref([])
const rules = ref([])
const devices = ref([])
const loadingEvents = ref(false)
const loadingRules = ref(false)
const statusFilter = ref('active')
const deviceFilter = ref('')
const activeTotal = computed(() => alarmStore.activeCount)

const ruleVisible = ref(false)
const savingRule = ref(false)
const editing = reactive({ deviceId: '', point: '', high: null, low: null, enabled: true, _origin: null })

function fmt(v) {
  if (v == null) return '—'
  return Number.isInteger(v) ? String(v) : Number(v).toFixed(4).replace(/0+$/, '').replace(/\.$/, '')
}

async function loadDevices() {
  const { data } = await api.get('/devices')
  devices.value = data.devices || []
}

function pointsOf(deviceId) {
  return devices.value.find((d) => d.id === deviceId)?.points || []
}

async function loadEvents() {
  loadingEvents.value = true
  try {
    const params = {}
    if (statusFilter.value !== 'all') params.status = statusFilter.value
    if (deviceFilter.value) params.deviceId = deviceFilter.value
    params.limit = 200
    const { data } = await api.get('/alarms', { params })
    events.value = data.events || []
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '加载报警失败')
  } finally {
    loadingEvents.value = false
  }
}

async function loadRules() {
  loadingRules.value = true
  try {
    const { data } = await api.get('/alarm-rules')
    rules.value = data.rules || []
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '加载规则失败')
  } finally {
    loadingRules.value = false
  }
}

async function clearOne(row) {
  try {
    await api.post(`/alarms/${row.id}/clear`)
    ElMessage.success('报警已消除')
    await Promise.all([loadEvents(), alarmStore.refreshSummary()])
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '消除失败')
  }
}

function openRule(row) {
  if (row) {
    Object.assign(editing, {
      deviceId: row.deviceId, point: row.point,
      high: row.high ?? null, low: row.low ?? null, enabled: row.enabled, _origin: row
    })
  } else {
    Object.assign(editing, { deviceId: '', point: '', high: null, low: null, enabled: true, _origin: null })
  }
  ruleVisible.value = true
}

function onDeviceChange() {
  editing.point = ''
}

async function saveRule() {
  if (!editing.deviceId || !editing.point) {
    ElMessage.warning('请选择设备和点位')
    return
  }
  if (editing.high == null && editing.low == null) {
    ElMessage.warning('高/低阈值至少填写一个')
    return
  }
  if (editing.high != null && editing.low != null && editing.low >= editing.high) {
    ElMessage.warning('低阈值必须小于高阈值')
    return
  }
  savingRule.value = true
  try {
    await api.put(`/devices/${editing.deviceId}/alarm-rules/${editing.point}`, {
      high: editing.high, low: editing.low, enabled: editing.enabled
    })
    ElMessage.success('规则已保存')
    ruleVisible.value = false
    await loadRules()
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '保存失败')
  } finally {
    savingRule.value = false
  }
}

async function removeRule(row) {
  try {
    await api.delete(`/devices/${row.deviceId}/alarm-rules/${row.point}`)
    ElMessage.success('规则已删除')
    await Promise.all([loadRules(), loadEvents(), alarmStore.refreshSummary()])
  } catch (e) {
    ElMessage.error(e.response?.data?.error || '删除失败')
  }
}

onMounted(async () => {
  await loadDevices()
  await Promise.all([loadEvents(), loadRules()])
})
</script>

<style scoped>
.tab-badge {
  margin-left: 6px;
}
.tab-badge :deep(.el-badge__content) {
  transform: translateY(-2px) translateX(100%);
}
</style>
