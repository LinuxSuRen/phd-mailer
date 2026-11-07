<script setup>
import { computed, onMounted, reactive, ref } from 'vue'
import { ElMessage, ElMessageBox } from 'element-plus'
import { Search } from '@element-plus/icons-vue'
import {
  ApiError,
  getContacts,
  getConfig,
  updateConfig,
  sendEmails,
} from './api'

const contacts = ref([])
const loading = ref(false)
const sending = ref(false)
const keyword = ref('')
const selection = ref([])
const config = reactive({
  smtp_user: '',
  smtp_host: '',
  smtp_port: 465,
  smtp_configured: false,
  interval_seconds: 60,
  dry_run: true,
})
const configDialogVisible = ref(false)
const configForm = reactive({ smtp_user: '', smtp_auth_code: '', interval_seconds: 60 })

const statusText = {
  sent: '已发送',
  dry_run: '演练通过',
  skipped_duplicate: '重复已跳过',
  failed: '发送失败',
}

const filtered = computed(() => {
  const kw = keyword.value.trim().toLowerCase()
  if (!kw) return contacts.value
  return contacts.value.filter((c) =>
    [c.name, c.university, c.department, c.email, ...(c.tags || [])]
      .join(' ')
      .toLowerCase()
      .includes(kw),
  )
})

const pendingSelected = computed(
  () => selection.value.filter((c) => !c.sent).length,
)

async function loadAll() {
  loading.value = true
  try {
    const [list, cfg] = await Promise.all([getContacts(), getConfig()])
    contacts.value = Array.isArray(list) ? list : []
    Object.assign(config, cfg)
  } catch (err) {
    showError(err)
  } finally {
    loading.value = false
  }
}

function showError(err) {
  ElMessage.error(err instanceof ApiError ? err.message : '操作失败，请稍后重试')
}

function openConfig() {
  configForm.smtp_user = config.smtp_user
  configForm.smtp_auth_code = ''
  configForm.interval_seconds = config.interval_seconds
  configDialogVisible.value = true
}

async function saveConfig() {
  try {
    await updateConfig(configForm)
    ElMessage.success('配置已保存')
    configDialogVisible.value = false
    await loadAll()
  } catch (err) {
    showError(err)
  }
}

async function handleSend() {
  const ids = selection.value.filter((c) => !c.sent).map((c) => c.id)
  if (ids.length === 0) {
    ElMessage.warning('请先勾选待发送的导师')
    return
  }
  const modeText = config.dry_run ? '演练模式（不会真实发信）' : '真实发送'
  try {
    await ElMessageBox.confirm(
      `即将${modeText} ${ids.length} 封邮件，按优先级排序依次发出，相邻间隔 ${config.interval_seconds} 秒。已发送过的导师会被自动跳过。`,
      '确认发送',
      { confirmButtonText: '开始发送', cancelButtonText: '取消', type: 'info' },
    )
  } catch {
    return
  }
  sending.value = true
  try {
    const results = await sendEmails(ids, config.dry_run)
    const summary = { sent: 0, dry_run: 0, skipped_duplicate: 0, failed: 0 }
    for (const r of results || []) {
      summary[r.status] = (summary[r.status] || 0) + 1
    }
    const parts = [
      summary.sent && `成功 ${summary.sent} 封`,
      summary.dry_run && `演练 ${summary.dry_run} 封`,
      summary.skipped_duplicate && `跳过重复 ${summary.skipped_duplicate} 封`,
      summary.failed && `失败 ${summary.failed} 封`,
    ].filter(Boolean)
    if (summary.failed) {
      const firstFail = results.find((r) => r.status === 'failed')
      ElMessage.warning(`${parts.join('，')}。首条失败原因：${firstFail?.detail || '未知'}`)
    } else {
      ElMessage.success(parts.join('，'))
    }
    await loadAll()
  } catch (err) {
    showError(err)
  } finally {
    sending.value = false
  }
}

onMounted(loadAll)
</script>

<template>
  <div class="page">
    <header class="page-header">
      <h1 class="page-header__title">phd-mailer · 博士套磁邮件发送台</h1>
      <div style="display: flex; gap: 8px; align-items: center">
        <el-tag :type="config.dry_run ? 'warning' : 'danger'" effect="dark">
          {{ config.dry_run ? '演练模式' : '真实发送' }}
        </el-tag>
        <el-button @click="openConfig">SMTP 配置</el-button>
        <el-button
          type="primary"
          :loading="sending"
          :disabled="pendingSelected === 0"
          @click="handleSend"
        >
          发送选中的 {{ pendingSelected }} 封
        </el-button>
      </div>
    </header>

    <el-alert
      v-if="!config.smtp_configured"
      title="尚未配置 SMTP 授权码：演练可用，真实发送前请在「SMTP 配置」中填入 QQ 邮箱地址与授权码"
      type="warning"
      :closable="false"
      style="margin-bottom: 16px"
    />

    <el-input
      v-model="keyword"
      placeholder="按姓名、学校、学院、邮箱或标签筛选"
      :prefix-icon="Search"
      clearable
      style="margin-bottom: 12px"
    />

    <el-table
      :data="filtered"
      v-loading="loading"
      row-key="id"
      @selection-change="selection = $event"
    >
      <el-table-column type="selection" width="44" :selectable="(row) => !row.sent" />
      <el-table-column type="expand">
        <template #default="{ row }">
          <div class="email-body-preview">
            <p style="color: var(--color-text-dim); margin-top: 0">主题：{{ row.subject }}</p>
            {{ row.body }}
          </div>
        </template>
      </el-table-column>
      <el-table-column prop="name" label="导师" width="90" />
      <el-table-column prop="university" label="学校" min-width="150" />
      <el-table-column prop="department" label="院系" min-width="180" show-overflow-tooltip />
      <el-table-column prop="email" label="邮箱" min-width="200" class="mono">
        <template #default="{ row }">
          <span class="mono">{{ row.email }}</span>
        </template>
      </el-table-column>
      <el-table-column label="标签" min-width="140">
        <template #default="{ row }">
          <el-tag v-for="t in row.tags" :key="t" size="small" style="margin-right: 4px">
            {{ t }}
          </el-tag>
        </template>
      </el-table-column>
      <el-table-column label="状态" width="110">
        <template #default="{ row }">
          <el-tag v-if="row.sent" type="success">已发送</el-tag>
          <el-tag v-else>待发送</el-tag>
        </template>
      </el-table-column>
      <el-table-column label="发送时间" width="160">
        <template #default="{ row }">
          <span v-if="row.sent" class="mono text-dim">
            {{ new Date(row.sent_at).toLocaleString('zh-CN') }}
          </span>
        </template>
      </el-table-column>
    </el-table>

    <p class="text-dim" style="margin-top: 12px">
      共 {{ contacts.length }} 位导师，已发送
      {{ contacts.filter((c) => c.sent).length }} 位。同一导师（含同一邮箱）永远只会收到一封邮件。
    </p>

    <el-dialog v-model="configDialogVisible" title="SMTP 配置（腾讯 QQ 邮箱）" width="480px">
      <el-form label-width="120px">
        <el-form-item label="SMTP 服务器">
          <span class="mono">{{ config.smtp_host }}:{{ config.smtp_port }}</span>
        </el-form-item>
        <el-form-item label="QQ 邮箱地址">
          <el-input v-model="configForm.smtp_user" placeholder="例如 someone@qq.com" />
        </el-form-item>
        <el-form-item label="SMTP 授权码">
          <el-input
            v-model="configForm.smtp_auth_code"
            type="password"
            show-password
            :placeholder="config.smtp_configured ? '已配置，留空则不修改' : '在 QQ 邮箱设置中生成'"
          />
        </el-form-item>
        <el-form-item label="发送间隔（秒）">
          <el-input-number v-model="configForm.interval_seconds" :min="0" :max="3600" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="configDialogVisible = false">取消</el-button>
        <el-button type="primary" @click="saveConfig">保存</el-button>
      </template>
    </el-dialog>
  </div>
</template>
