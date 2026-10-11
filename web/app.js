// Dynamic Multi-Language Client Script
let currentLang = localStorage.getItem('mega_lang') || navigator.language.slice(0, 2) || 'en';
if (!['en', 'pt', 'es'].includes(currentLang)) {
  currentLang = 'en';
}

let dict = {};
let accountsList = [];

// Dynamic string resolver - NO fixed strings
function t(key, ...args) {
  let val = dict[key] || key;
  if (args.length > 0) {
    args.forEach((arg, i) => {
      val = val.replace('%s', arg).replace('%d', arg);
    });
  }
  return val;
}

// Fetch language dictionary dynamically from Go server
async function loadLanguage(lang) {
  try {
    const res = await fetch(`/api/i18n?lang=${encodeURIComponent(lang)}`);
    if (res.ok) {
      dict = await res.json();
      currentLang = lang;
      localStorage.setItem('mega_lang', lang);
      document.getElementById('langSelect').value = lang;
      applyTranslations();
      renderAccountsTable();
    }
  } catch (err) {
    console.error('Failed to load translations:', err);
  }
}

function applyTranslations() {
  document.querySelectorAll('[data-i18n]').forEach(el => {
    const key = el.getAttribute('data-i18n');
    if (dict[key]) {
      el.textContent = dict[key];
    }
  });

  document.querySelectorAll('[data-i18n-placeholder]').forEach(el => {
    const key = el.getAttribute('data-i18n-placeholder');
    if (dict[key]) {
      el.setAttribute('placeholder', dict[key]);
    }
  });

  const simulateTextarea = document.getElementById('simulatePayload');
  if (simulateTextarea && dict['lbl_webhook_body_placeholder']) {
    simulateTextarea.setAttribute('placeholder', dict['lbl_webhook_body_placeholder']);
  }

  if (dict['app_title']) {
    document.title = dict['app_title'];
  }
}

// Format dates
function formatDate(isoStr) {
  if (!isoStr || isoStr.startsWith('0001-01-01')) return '--';
  try {
    const d = new Date(isoStr);
    return d.toLocaleString(currentLang === 'pt' ? 'pt-BR' : currentLang === 'es' ? 'es-ES' : 'en-US', {
      month: 'short', day: 'numeric', hour: '2-digit', minute: '2-digit'
    });
  } catch {
    return isoStr;
  }
}

// Load System Status
async function refreshStatus() {
  try {
    const res = await fetch('/api/status');
    if (!res.ok) return;
    const data = await res.json();

    document.getElementById('valTotalAccounts').textContent = data.total || 0;
    document.getElementById('valActiveAccounts').textContent = data.active || 0;
    document.getElementById('valPendingAccounts').textContent = data.pending || 0;
    
    if (data.next_run && !data.next_run.startsWith('0001-01-01')) {
      document.getElementById('valNextKeepalive').textContent = formatDate(data.next_run);
    } else {
      document.getElementById('valNextKeepalive').textContent = '--';
    }

    const intervalText = `${t('lbl_keepalive_interval')}: ${data.interval_days || 30}d`;
    document.getElementById('valKeepaliveInterval').textContent = intervalText;

    const patternText = `${data.pattern || 'mega%d'}@${data.domain || 'domain'}`;
    document.getElementById('valPatternDomain').textContent = patternText;
    document.getElementById('hintPattern').textContent = `${t('lbl_pattern_preview')}: ${data.next_email || patternText}`;
    document.getElementById('badgeNextEmail').textContent = data.next_email || patternText;

    const webhookFullUrl = `${window.location.origin}/api/webhooks/mailgun`;
    document.getElementById('webhookUrlInput').value = webhookFullUrl;

  } catch (err) {
    console.error('Failed to fetch status:', err);
  }
}

// Load Accounts List
async function refreshAccounts() {
  try {
    const res = await fetch('/api/accounts');
    if (!res.ok) return;
    accountsList = await res.json() || [];
    renderAccountsTable();
  } catch (err) {
    console.error('Failed to fetch accounts:', err);
  }
}

function renderAccountsTable() {
  const tbody = document.getElementById('accountsTableBody');
  const emptyState = document.getElementById('emptyAccountsState');
  const query = (document.getElementById('tableSearch').value || '').toLowerCase().trim();

  const filtered = accountsList.filter(acc => {
    if (!query) return true;
    return (acc.email && acc.email.toLowerCase().includes(query)) ||
           (acc.status && acc.status.toLowerCase().includes(query)) ||
           (acc.purpose && acc.purpose.toLowerCase().includes(query));
  });

  if (filtered.length === 0) {
    tbody.innerHTML = '';
    emptyState.style.display = 'block';
    return;
  }

  emptyState.style.display = 'none';
  tbody.innerHTML = filtered.map(acc => {
    const badgeClass = acc.status === 'verified' ? 'badge-verified' :
                       acc.status === 'pending' ? 'badge-pending' : 'badge-failed';
    const badgeKey = acc.status === 'verified' ? 'badge_verified' :
                     acc.status === 'pending' ? 'badge_pending' : 'badge_failed';
    const badgeLabel = t(badgeKey);

    return `
      <tr>
        <td>
          <div style="font-weight: 600; color: var(--text-primary);">${escapeHtml(acc.email)}</div>
          ${acc.error_message ? `<div style="font-size: 0.72rem; color: var(--accent-rose);">${escapeHtml(acc.error_message)}</div>` : ''}
        </td>
        <td>
          <span class="code-pill">${escapeHtml(acc.mega_password || '******')}</span>
        </td>
        <td>
          <span class="badge ${badgeClass}">${badgeLabel}</span>
        </td>
        <td>
          <div>${formatDate(acc.last_login_at)}</div>
          ${acc.last_login_status ? `<span style="font-size: 0.72rem; color: ${acc.last_login_status === 'success' ? 'var(--accent-emerald)' : 'var(--accent-rose)'};">${escapeHtml(acc.last_login_status)}</span>` : ''}
        </td>
        <td style="font-size: 0.78rem;">${formatDate(acc.created_at)}</td>
        <td>
          <button class="btn btn-secondary btn-sm" onclick="copyCreds('${escapeHtml(acc.email)}', '${escapeHtml(acc.mega_password)}')">
            ${t('btn_copy')}
          </button>
        </td>
      </tr>
    `;
  }).join('');
}

function copyCreds(email, pass) {
  navigator.clipboard.writeText(`${email}:${pass}`);
  alert(`${email} copied!`);
}

function escapeHtml(str) {
  if (!str) return '';
  return str.replace(/[&<>"']/g, m => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  })[m]);
}

// Load Activity Logs
async function refreshLogs() {
  try {
    const res = await fetch('/api/logs');
    if (!res.ok) return;
    const logs = await res.json() || [];
    const box = document.getElementById('logsBox');
    box.innerHTML = logs.map(l => `
      <div class="log-entry">
        <span class="log-time">${l.time || ''}</span>
        <span class="log-level ${l.level}">${l.level}</span>
        <span class="log-msg">${escapeHtml(l.msg)}</span>
      </div>
    `).join('');
  } catch (err) {
    console.error('Failed to fetch logs:', err);
  }
}

// Event Listeners
document.addEventListener('DOMContentLoaded', async () => {
  await loadLanguage(currentLang);
  await refreshStatus();
  await refreshAccounts();
  await refreshLogs();

  // Language switch
  document.getElementById('langSelect').addEventListener('change', (e) => {
    loadLanguage(e.target.value);
  });

  // Refresh button
  document.getElementById('btnRefresh').addEventListener('click', () => {
    refreshStatus();
    refreshAccounts();
    refreshLogs();
  });

  // Search input
  document.getElementById('tableSearch').addEventListener('input', renderAccountsTable);

  // Copy webhook URL
  document.getElementById('btnCopyWebhook').addEventListener('click', () => {
    const input = document.getElementById('webhookUrlInput');
    input.select();
    navigator.clipboard.writeText(input.value);
    const btn = document.getElementById('btnCopyWebhook');
    btn.textContent = t('btn_copied');
    setTimeout(() => { btn.textContent = t('btn_copy'); }, 2000);
  });

  // Generate form submit
  document.getElementById('genForm').addEventListener('submit', async (e) => {
    e.preventDefault();
    const count = parseInt(document.getElementById('inputCount').value, 10) || 1;
    const password = document.getElementById('inputPassword').value.trim();
    const btn = document.getElementById('btnSubmitGen');
    const origText = btn.innerHTML;
    btn.disabled = true;
    btn.textContent = t('btn_generating');

    try {
      const res = await fetch('/api/accounts/generate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ count, password })
      });
      const data = await res.json();
      if (!res.ok) {
        alert(data.error || 'Failed to start generation');
      } else {
        await refreshStatus();
        await refreshAccounts();
        await refreshLogs();
      }
    } catch (err) {
      alert(err.message);
    } finally {
      btn.disabled = false;
      btn.innerHTML = origText;
    }
  });

  // Keep-Alive trigger
  document.getElementById('btnKeepAliveNow').addEventListener('click', async () => {
    try {
      const res = await fetch('/api/accounts/keepalive', { method: 'POST' });
      if (res.ok) {
        refreshLogs();
      }
    } catch (err) {
      console.error(err);
    }
  });

  // Simulate webhook
  document.getElementById('btnSimulate').addEventListener('click', async () => {
    const payload = document.getElementById('simulatePayload').value.trim();
    if (!payload) return;
    try {
      const res = await fetch('/api/webhooks/simulate', {
        method: 'POST',
        headers: { 'Content-Type': 'application/json' },
        body: JSON.stringify({ body: payload })
      });
      const data = await res.json();
      if (res.ok) {
        document.getElementById('simulatePayload').value = '';
        await refreshAccounts();
        await refreshLogs();
      } else {
        alert(data.error || 'Simulation error');
      }
    } catch (err) {
      alert(err.message);
    }
  });

  // Clear logs button
  document.getElementById('btnClearLogs').addEventListener('click', () => {
    document.getElementById('logsBox').innerHTML = '';
  });

  // Periodic polling every 5s
  setInterval(() => {
    refreshStatus();
    refreshAccounts();
    refreshLogs();
  }, 5000);
});
