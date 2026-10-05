<template>
  <div class="page-shell">
    <header class="topbar">
      <div>
        <h1>Workflow Request Manager</h1>
        <p>n8n + Vue + Go + Solace sample</p>
      </div>
      <button class="primary" @click="loadRequests">Refresh</button>
    </header>

    <section class="panel form-panel">
      <h2>Create request</h2>
      <div class="form-grid">
        <label>
          Filename
          <input v-model="filename" placeholder="a=example-file.txt" />
        </label>
        <label>
          Twig template override
          <textarea v-model="twigTemplate" rows="4" placeholder="{{ filename }} | {{ status }}"></textarea>
        </label>
      </div>
      <button class="primary" @click="createRequest">Create request and trigger workflow</button>
      <p v-if="message" class="message">{{ message }}</p>
    </section>

    <section class="panel">
      <h2>Recent requests</h2>
      <div v-if="loading">Loading…</div>
      <table v-else>
        <thead>
          <tr>
            <th>ID</th>
            <th>Filename</th>
            <th>Status</th>
            <th>Workflow</th>
            <th>Run ID</th>
            <th>Actions</th>
          </tr>
        </thead>
        <tbody>
          <tr v-for="req in requests" :key="req.id">
            <td>{{ req.id }}</td>
            <td>{{ req.filename }}</td>
            <td>
              <span :class="['badge', req.status]">{{ req.status }}</span>
            </td>
            <td>
              <a v-if="req.workflow_url" :href="req.workflow_url" target="_blank">Open workflow</a>
              <span v-else>—</span>
            </td>
            <td>{{ req.run_id || '—' }}</td>
            <td>
              <button @click="openWorkflow(req.workflow_url)">View</button>
              <button @click="runTemporaryWorkflow(req.id)">Temporary run</button>
            </td>
          </tr>
        </tbody>
      </table>
    </section>
  </div>
</template>

<script setup>
import { ref, onMounted } from 'vue'

const apiBase = 'http://localhost:8081'
const requests = ref([])
const loading = ref(false)
const filename = ref('a=demo-file.txt')
const twigTemplate = ref('{{ filename }} | {{ status }}')
const message = ref('')

async function loadRequests() {
  loading.value = true
  try {
    const res = await fetch(`${apiBase}/api/requests`)
    requests.value = await res.json()
  } catch (err) {
    message.value = 'Could not load requests from backend.'
    console.error(err)
  } finally {
    loading.value = false
  }
}

async function createRequest() {
  try {
    const res = await fetch(`${apiBase}/api/requests`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        filename: filename.value,
        twig_template: twigTemplate.value
      })
    })

    if (!res.ok) {
      throw new Error('Failed to create request')
    }

    const data = await res.json()
    message.value = `Request created: ${data.filename} (id=${data.id})`
    await loadRequests()
  } catch (err) {
    message.value = 'There was an error creating the request.'
    console.error(err)
  }
}

async function runTemporaryWorkflow(id) {
  try {
    const res = await fetch(`${apiBase}/api/requests/${id}/temporary-workflow`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        twig_template: twigTemplate.value,
        test_data: {
          filename: `a=temp-${id}.txt`,
          status: 'success',
          request_id: id
        }
      })
    })

    if (!res.ok) {
      throw new Error('Temporary workflow run failed')
    }

    const data = await res.json()
    message.value = `Temporary workflow run started: ${data.workflow_id}`
    await loadRequests()
  } catch (err) {
    message.value = 'Temporary workflow run failed.'
    console.error(err)
  }
}

function openWorkflow(url) {
  if (url) window.open(url, '_blank')
}

onMounted(loadRequests)
</script>
