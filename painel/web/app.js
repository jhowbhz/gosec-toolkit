'use strict';
// painel — lógica da UI (sem framework). O token vem na URL (?token=), é
// guardado no sessionStorage e removido da barra de endereço; vai como header
// nas chamadas /api e como ?token= no stream (SSE não manda header).

const $ = (s) => document.querySelector(s);
let TOKEN = '';
let TOOLS = [];
let RUNS = [];
let atual = null;     // ferramenta selecionada
let esRun = null;     // EventSource ativo
let runAtivo = null;  // id do run em andamento

function pegarToken() {
  const u = new URL(location.href);
  const t = u.searchParams.get('token');
  if (t) {
    sessionStorage.setItem('painel_token', t);
    u.searchParams.delete('token');
    history.replaceState(null, '', u.pathname + u.search);
  }
  return sessionStorage.getItem('painel_token') || '';
}

async function api(caminho, opts = {}) {
  const r = await fetch(caminho, {
    ...opts,
    headers: { 'X-Painel-Token': TOKEN, 'Content-Type': 'application/json', ...(opts.headers || {}) },
  });
  if (!r.ok) throw new Error((await r.text()) || ('HTTP ' + r.status));
  return r.status === 204 ? null : r.json();
}

function mostrarErro(msg) {
  const e = $('#erro');
  e.textContent = msg;
  e.hidden = !msg;
}

// --- ferramentas ---
async function carregarTools() {
  try {
    TOOLS = await api('/api/tools') || [];
    const box = $('#toolsLista');
    box.innerHTML = '';
    for (const t of TOOLS) {
      const b = document.createElement('button');
      b.className = 'tool-item';
      b.type = 'button';
      b.setAttribute('aria-pressed', 'false');
      b.innerHTML = `<b></b><small></small>`;
      b.querySelector('b').textContent = t.nome;
      b.querySelector('small').textContent = t.descricao;
      b.onclick = () => selecionar(t.id);
      box.appendChild(b);
    }
  } catch (e) { mostrarErro('Falha ao carregar ferramentas: ' + e.message); }
}

function selecionar(id) {
  atual = TOOLS.find((t) => t.id === id);
  document.querySelectorAll('.tool-item').forEach((b, i) =>
    b.setAttribute('aria-pressed', String(TOOLS[i].id === id)));
  $('#toolNome').textContent = atual.nome;
  $('#toolDesc').textContent = atual.descricao;
  renderForm();
}

function optsRunRef(campo) {
  // opções: dirs de run (run-ref dir) ou arquivos de artefato (run-ref arquivo)
  const out = [{ v: '', t: '— nenhum —' }];
  for (const r of RUNS) {
    if (campo.run_tipo === 'dir') {
      out.push({ v: r.dir, t: `${r.run.tool} · ${r.run.id}` });
    } else {
      for (const a of (r.artefatos || [])) out.push({ v: r.dir + '/' + a, t: `${r.run.id}/${a}` });
    }
  }
  return out;
}

function renderForm() {
  const form = $('#form');
  form.innerHTML = '';
  form.hidden = false;
  for (const c of atual.campos || []) {
    const wrap = document.createElement('div');
    wrap.className = 'campo' + (c.tipo === 'bool' ? ' check' : '') +
      (c.repetivel || c.tipo === 'texto' || c.tipo === 'run-ref' ? ' full' : '');
    const id = 'f_' + c.flag;
    const label = `<label for="${id}">${c.label || c.flag}${c.required ? ' *' : ''}</label>`;
    const ajuda = c.ajuda ? `<span class="ajuda">${c.ajuda}</span>` : '';

    if (c.tipo === 'bool') {
      wrap.innerHTML = `<input type="checkbox" id="${id}" data-flag="${c.flag}" ${c.default === 'true' ? 'checked' : ''}><label for="${id}">${c.label || c.flag}</label>`;
    } else if (c.repetivel) {
      wrap.innerHTML = `${label}<textarea id="${id}" data-flag="${c.flag}" data-rep="1" placeholder="um por linha"></textarea>${ajuda}`;
    } else if (c.tipo === 'enum') {
      const ops = (c.opcoes || []).map((o) => `<option ${o === c.default ? 'selected' : ''}>${o}</option>`).join('');
      wrap.innerHTML = `${label}<select id="${id}" data-flag="${c.flag}">${ops}</select>${ajuda}`;
    } else if (c.tipo === 'run-ref') {
      const ops = optsRunRef(c).map((o) => `<option value="${o.v}">${o.t}</option>`).join('');
      wrap.innerHTML = `${label}<select id="${id}" data-flag="${c.flag}">${ops}</select>${ajuda}`;
    } else {
      const tipo = c.tipo === 'int' ? 'number' : 'text';
      wrap.innerHTML = `${label}<input type="${tipo}" id="${id}" data-flag="${c.flag}" value="${c.default || ''}">${ajuda}`;
    }
    form.appendChild(wrap);
  }
  const acoes = document.createElement('div');
  acoes.className = 'acoes';
  acoes.innerHTML = `<button type="submit" class="btn primario">Rodar ${atual.nome}</button>`;
  form.appendChild(acoes);
  form.onsubmit = rodar;
}

function coletarArgs() {
  const args = {};
  $('#form').querySelectorAll('[data-flag]').forEach((el) => {
    const flag = el.dataset.flag;
    if (el.type === 'checkbox') {
      args[flag] = [el.checked ? 'true' : 'false'];
    } else if (el.dataset.rep) {
      const vs = el.value.split('\n').map((x) => x.trim()).filter(Boolean);
      if (vs.length) args[flag] = vs;
    } else if (el.value !== '') {
      args[flag] = [el.value];
    }
  });
  return args;
}

// --- execução + stream ---
async function rodar(ev) {
  ev.preventDefault();
  mostrarErro('');
  try {
    const { id } = await api('/api/run', { method: 'POST', body: JSON.stringify({ tool: atual.id, args: coletarArgs() }) });
    runAtivo = id;
    const con = $('#console');
    con.textContent = '';
    $('#btnParar').hidden = false;
    abrirStream(id);
    carregarRuns();
  } catch (e) { mostrarErro('Falha ao rodar: ' + e.message); }
}

function abrirStream(id) {
  if (esRun) esRun.close();
  const con = $('#console');
  esRun = new EventSource(`/api/runs/${id}/stream?token=${encodeURIComponent(TOKEN)}`);
  esRun.onmessage = (e) => {
    con.textContent += e.data + '\n';
    con.scrollTop = con.scrollHeight;
  };
  esRun.addEventListener('fim', () => finalizarStream());
  esRun.onerror = () => finalizarStream();
}

function finalizarStream() {
  if (esRun) { esRun.close(); esRun = null; }
  $('#btnParar').hidden = true;
  runAtivo = null;
  carregarRuns();
}

$('#btnParar').onclick = async () => {
  if (!runAtivo) return;
  try { await api(`/api/runs/${runAtivo}/stop`, { method: 'POST' }); } catch (e) { mostrarErro(e.message); }
};

// --- execuções + artefatos ---
async function carregarRuns() {
  try {
    RUNS = await api('/api/runs') || [];
    const box = $('#runsLista');
    if (!RUNS.length) { box.innerHTML = '<p class="vazio-painel">Nenhuma ainda.</p>'; return; }
    box.innerHTML = '';
    for (const r of RUNS) {
      const b = document.createElement('button');
      b.className = 'run-item';
      b.type = 'button';
      const hora = new Date(r.run.inicio).toLocaleTimeString('pt-BR', { hour: '2-digit', minute: '2-digit' });
      b.innerHTML = `<span class="nome"></span><time></time><span class="badge ${r.run.status}"></span>`;
      b.querySelector('.nome').textContent = r.run.tool;
      b.querySelector('time').textContent = hora;
      b.querySelector('.badge').textContent = r.run.status;
      b.onclick = () => verRun(r);
      box.appendChild(b);
    }
  } catch (e) { mostrarErro('Falha ao listar execuções: ' + e.message); }
}

function verRun(r) {
  const cx = $('#artefatos');
  const ul = $('#artefatosLista');
  ul.innerHTML = '';
  const arts = r.artefatos || [];
  if (!arts.length) { cx.hidden = true; return; }
  for (const a of arts) {
    const li = document.createElement('li');
    const link = document.createElement('a');
    link.textContent = a;
    link.href = `/api/runs/${r.run.id}/file?path=${encodeURIComponent(a)}&token=${encodeURIComponent(TOKEN)}`;
    link.target = '_blank';
    li.appendChild(link);
    ul.appendChild(li);
  }
  cx.hidden = false;
}

// --- boot ---
TOKEN = pegarToken();
if (!TOKEN) {
  mostrarErro('Sem token. Abra o painel pela URL que o terminal imprimiu (com ?token=…).');
} else {
  carregarTools();
  carregarRuns();
  setInterval(() => { if (!runAtivo) carregarRuns(); }, 5000);
}
