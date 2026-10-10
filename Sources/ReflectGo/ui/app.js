'use strict';
const $ = id => document.getElementById(id);
const storeKey = 'reflect-go-session-' + location.port;
const clientNames = {codex: 'Codex CLI', claude: 'Claude CLI', opencode: 'OpenCode CLI'};
let token = location.hash.slice(1) || sessionStorage.getItem(storeKey) || '';
if (token) { sessionStorage.setItem(storeKey, token); history.replaceState(null, '', location.pathname); }
let inv = null, engine = null, selected = '', enabled = null, loading = false, initialized = false, closed = false;
let lastReport = null, catalog = [], download = null, providerSignature = '', chatController = null, chatReader = null, chatStopping = false, messages = [];
let launchPrefs = {project: '', clients: {}}, prefsLoaded = false, clientPacks = {}, launching = '', serviceStarting = false;
let prefsSave = Promise.resolve(), runSave = Promise.resolve(), integrations = [], integrationsLoading = false;
let settingsRestored = false, preferencesLoad = null, integrationsSeen = false;
const gib = n => (n / 1073741824).toFixed(2) + ' GiB';
async function api(path, body) {
  const options = {headers: {Authorization: 'Bearer ' + token}};
  if (body !== undefined) { options.method = 'POST'; options.headers['Content-Type'] = 'application/json'; options.body = JSON.stringify(body); }
  const r = await fetch('/api/' + path, options);
  if (!r.ok) throw Error(await r.text());
  return r.json();
}
function applyTheme(theme) {
  document.documentElement.dataset.theme = theme;
  // The native window follows the same setting without polling files on the
  // drive. Browsers have no native bridge and simply use the CSS theme.
  try { window.webkit?.messageHandlers?.appearance?.postMessage(theme); }
  catch (e) { console.error('Reflect Go could not update the native appearance', e); }
}
function error(e) { $('error').textContent = e.message || String(e); $('error').hidden = false; }
function clearError() { $('error').hidden = true; }
function hostKey() { return inv ? [inv.hardware.os, inv.hardware.architecture, inv.hardware.cpu, inv.hardware.physicalMemoryBytes].join('|') : ''; }
function settings() {
  return {host: hostKey(), model: selected, enabled: enabled === null ? null : [...enabled], context: Number($('context').value), backend: $('backend').value, temperature: Number($('temperature').value), threads: Number($('threads').value), port: Number($('port').value)};
}
function setSettings(s) {
  selected = s.model || '';
  enabled = s.enabled == null ? null : new Set(s.enabled);
  for (const key of ['context', 'backend', 'temperature', 'threads', 'port']) if (s[key] !== undefined) $(key).value = s[key];
}
function friendlyName(model) { return catalog.find(m => m.filename === model.name)?.name || model.name.replace(/\.gguf$/i, '').replace(/_/g, ' '); }
function sharedModels() { return (inv?.models || []).filter(m => enabled === null ? m.eligible : enabled.has(m.id)); }
function clientPreference(client) { return launchPrefs.clients[client] || {enabled: null, defaultModel: ''}; }
function offeredModels(client) {
  const pref = clientPreference(client);
  return pref.enabled == null ? sharedModels() : (inv?.models || []).filter(m => pref.enabled.includes(m.id));
}
function updateClientCards() {
  if (!inv) return;
  const shared = sharedModels();
  $('enabled-summary').textContent = shared.length + ' available';
  $('home-model-summary').textContent = shared.length + ' model' + (shared.length === 1 ? '' : 's') + ' available';
  $('empty-library').hidden = !!inv.models.length;
  for (const client of Object.keys(clientNames)) {
    const models = offeredModels(client), pref = clientPreference(client);
    const chosen = models.find(m => m.id === pref.defaultModel) || (!pref.defaultModel && models.find(m => m.name === selected));
    $(client + '-models').textContent = models.length ? models.length + ' model' + (models.length === 1 ? '' : 's') + ' · ' + (chosen ? friendlyName(chosen) : pref.defaultModel ? 'Default unavailable' : 'Automatic') : 'No models selected';
    $(client + '-models').title = chosen ? 'Starting model: ' + friendlyName(chosen) : $(client + '-models').textContent;
    $(client + '-availability').textContent = 'Client pack not on this drive';
    $(client + '-availability').hidden = !!clientPacks[client];
  }
}
async function loadLaunchPreferences() {
  if (prefsLoaded) return;
  if (preferencesLoad) return preferencesLoad;
  preferencesLoad = (async () => {
    const saved = await api('launch-preferences');
    launchPrefs = {project: saved.project || '', clients: saved.clients || {}};
    $('project').value = launchPrefs.project; prefsLoaded = true;
    if (saved.projectWarning) { $('project-warning').textContent = saved.projectWarning; $('project-warning').hidden = false; $('launch-options').open = true; }
    controls(); updateClientCards();
  })();
  try { await preferencesLoad; } finally { preferencesLoad = null; }
}
async function syncEngineSettings() {
  // A successful launch may raise context for its client. Refresh the form only
  // while that launch owns the controls, never over someone's stopped edits.
  try {
    engine = (await api('status')).engine;
    setSettings(engine.savedSettings || engine.settings);
    settingsRestored = true;
    await refresh(); controls();
  } catch (e) {
    console.error('Reflect Go could not refresh settings after opening an app', e);
    error(Error('The app was requested, but its updated settings could not be refreshed: ' + e.message));
  }
}
function savePreferences() {
  launchPrefs.project = $('project').value.trim();
  const snapshot = JSON.parse(JSON.stringify(launchPrefs));
  prefsSave = prefsSave.catch(() => {}).then(() => api('launch-preferences', snapshot));
  prefsSave.then(() => { $('model-save-status').textContent = 'App preferences saved on your drive.'; }, e => { $('model-save-status').textContent = 'Preferences could not be saved.'; error(e); });
  return prefsSave;
}
function saveRunSettings() {
  const snapshot = settings();
  runSave = runSave.catch(() => {}).then(() => api('run-settings', snapshot));
  runSave.then(() => { $('model-save-status').textContent = 'Shared settings saved on your drive.'; }, e => { $('model-save-status').textContent = 'Shared settings could not be saved.'; error(e); });
  return runSave;
}
function modelsChanged(isShared) {
  renderModels(); updateClientCards(); controls();
  (isShared ? saveRunSettings() : savePreferences()).catch(() => {});
}
function renderModels() {
  const list = $('models'); list.replaceChildren();
  if (!inv) return;
  const client = $('model-client').value, isShared = client === 'all', pref = isShared ? null : clientPreference(client);
  const inherited = !isShared && pref.enabled == null;
  const locked = !!launching || loading || (isShared && !!(engine?.active || engine?.preparing));
  $('client-model-options').hidden = isShared;
  $('shared-model-options').hidden = !isShared;
  $('all-suitable-models').checked = enabled === null;
  $('all-suitable-models').disabled = locked;
  $('automatic-default').hidden = !isShared;
  $('automatic-default').disabled = locked || !selected;
  $('automatic-default').textContent = selected ? 'Use an automatic default' : 'Automatic default selected';
  $('inherit-models').checked = inherited;
  $('inherit-models').disabled = locked;
  $('model-service-settings').hidden = !isShared || !(engine?.active || engine?.preparing);
  $('model-editor-help').textContent = isShared && (engine?.active || engine?.preparing) ? 'Stop the local service in Settings before changing shared models. Finish client tasks first.' : isShared ? 'Shared defaults apply to apps that use the shared selection. Only one model loads at a time, when requested.' : 'Changes apply when you next open ' + clientNames[client] + '. The local service must already offer any models added while it is running.';
  if (!isShared) {
    const select = $('client-default'); select.replaceChildren(new Option('Automatic', ''));
    for (const m of offeredModels(client)) select.add(new Option(friendlyName(m) + (m.eligible ? '' : ' · above memory estimate'), m.id));
    if (pref.defaultModel && ![...select.options].some(o => o.value === pref.defaultModel)) select.add(new Option('Unavailable starting model · choose another', pref.defaultModel));
    select.value = pref.defaultModel || '';
    select.disabled = locked;
  }
  for (const m of inv.models) {
    const row = document.createElement('div'); row.className = 'model-row';
    const label = document.createElement('label'), check = document.createElement('input'); check.type = 'checkbox';
    check.checked = isShared || inherited ? sharedModels().some(item => item.id === m.id) : pref.enabled.includes(m.id);
    check.disabled = locked || inherited || (!m.eligible && !check.checked);
    check.onchange = () => {
      if (isShared) {
        if (enabled === null) enabled = new Set(sharedModels().map(item => item.id));
        if (check.checked) enabled.add(m.id); else enabled.delete(m.id);
        if (selected === m.name && !check.checked) selected = '';
      } else {
        const values = new Set(pref.enabled); if (check.checked) values.add(m.id); else values.delete(m.id);
        launchPrefs.clients[client] = {...pref, enabled: [...values], defaultModel: pref.defaultModel === m.id && !check.checked ? '' : pref.defaultModel};
      }
      modelsChanged(isShared);
    };
    const name = document.createElement('strong'); name.textContent = friendlyName(m); label.append(check, name);
    const note = document.createElement('p'); note.textContent = gib(m.bytes) + ' on disk · ~' + gib(m.estimatedBytes) + ' estimated · ' + m.purpose;
    const fit = document.createElement('p'); fit.className = 'fine'; fit.textContent = m.issue || (m.eligible ? 'Within the conservative memory estimate' : 'Above the current memory budget. Lower context or use a smaller model.');
    row.append(label, note, fit);
    if (isShared) {
      const button = document.createElement('button'); button.textContent = selected === m.name ? 'Default model' : 'Use as default'; button.disabled = locked || !m.eligible;
      button.onclick = () => { selected = m.name; if (enabled !== null) enabled.add(m.id); modelsChanged(true); };
      row.append(button);
    }
    list.append(row);
  }
  if (!inv.models.length) list.textContent = 'No models installed. Choose Add models to download or import one.';
}
function controls() {
  const active = !!engine?.active, preparing = !!engine?.preparing, pending = !!launching || serviceStarting;
  $('start').hidden = active || preparing;
  $('stop').hidden = !active && !preparing;
  $('stop').disabled = !!engine?.stopping || pending;
  $('start').disabled = loading || pending || !inv?.hardware.backend.available;
  $('chat-start').hidden = active || preparing;
  $('chat-start').disabled = $('start').disabled;
  $('settings').disabled = active || preparing || loading || pending;
  $('refresh').disabled = active || preparing || loading || pending;
  $('project').disabled = pending || !prefsLoaded; $('task').disabled = pending; $('browse-project').disabled = pending || !prefsLoaded; $('clear-project').disabled = pending || !prefsLoaded;
  $('clear-project').hidden = !$('project').value;
  updateProjectSummary();
  $('model-client').disabled = pending;
  $('refresh-integrations').disabled = pending || integrationsLoading;
  document.querySelectorAll('[data-integration-action]').forEach(button => { button.disabled = pending || !initialized || !prefsLoaded || !inv || loading || !button.dataset.available; });
  $('test').disabled = !engine?.ready || engine?.testing || pending;
  $('settings-note').textContent = active ? 'Stop & unload to change shared models or runtime settings.' : '';
  $('dot').className = 'dot' + (engine?.ready ? ' ready' : active || preparing || pending ? ' active' : '');
  $('chat-send').disabled = !engine?.ready || !!chatController || chatStopping || !$('chat-model').value;
  $('chat-model').disabled = !!chatController; $('new-chat').disabled = !!chatController;
  for (const client of Object.keys(clientNames)) {
    $(client).disabled = !initialized || !prefsLoaded || !inv || loading || pending || !clientPacks[client] || !!engine?.stopping;
    $(client).textContent = launching === client ? 'Opening…' : 'Open';
  }
}
async function refresh() {
  if (loading) return false;
  loading = true; controls();
  try {
    inv = await api('inventory?context=' + $('context').value + '&backend=' + $('backend').value);
    const h = inv.hardware;
    $('cpu').textContent = h.cpu || h.architecture; $('platform').textContent = h.os + ' · ' + h.architecture;
    $('memory').textContent = gib(h.physicalMemoryBytes); $('budget').textContent = gib(h.screeningBudgetBytes) + ' screening budget';
    $('count').textContent = inv.models.length + ' models'; $('modelspath').textContent = inv.modelsPath;
    $('backend-detail').textContent = h.backend.available ? h.backend.kind.toUpperCase() + ' · ' + h.backend.device + (h.backend.reason ? ' — ' + h.backend.reason : '') : h.backend.reason;
    $('threads').max = h.cpuThreads; renderCatalog(); updateClientCards();
    return true;
  } catch (e) { console.error('Reflect Go model inventory refresh failed', e); error(e); return false; } finally { loading = false; renderModels(); controls(); }
}
async function loadProviderModels() {
  try {
    const data = await api('provider-models'), select = $('chat-model'), previous = select.value; select.replaceChildren();
    for (const m of data.data || []) { const option = document.createElement('option'); option.value = m.id; option.textContent = m.name || m.id; select.append(option); }
    if ([...select.options].some(o => o.value === previous)) select.value = previous;
    controls();
  } catch (e) { error(e); }
}
async function poll() {
  if (closed) return;
  try {
    const data = await api('status'), wasLocked = !!(engine?.active || engine?.preparing);
    engine = data.engine; const first = !initialized;
    if (!settingsRestored) { setSettings(engine.savedSettings || engine.settings); settingsRestored = true; }
    $('metal').disabled = !data.platform.startsWith('macos'); $('vulkan').disabled = data.platform.startsWith('macos');
    if (wasLocked !== !!(engine.active || engine.preparing)) renderModels();
    download = data.download; clientPacks = data.clients; $('connection').hidden = true;
    const g = data.gateway || {};
    $('status').textContent = engine.ready ? 'Local service running' : engine.active || engine.preparing ? engine.status : 'Ready when you are';
    $('detail').textContent = engine.ready ? 'Models load when an app requests them.' : engine.active || engine.preparing ? 'Starting the local service. Keep the drive connected.' : 'The local service starts when you open an app.';
    $('loading-stage').textContent = g.model ? g.stage + ': ' + g.model + (g.error ? ' — ' + g.error : '') : '';
    $('load-time').textContent = g.loadSeconds ? 'Last model load: ' + g.loadSeconds.toFixed(1) + ' seconds' : '';
    $('speed').textContent = engine.speed > 0 ? engine.speed.toFixed(1) : '—'; $('resident').textContent = engine.residentBytes ? gib(engine.residentBytes) : '—';
    $('clients').textContent = Object.keys(clientNames).map(n => clientNames[n] + ': ' + (data.clients[n] ? 'on this drive' : 'pack missing')).join(' · ');
    $('endpoint').textContent = engine.ready ? 'http://127.0.0.1:' + engine.settings.port : '';
    if (engine.error) error(Error(engine.error));
    const {log, ...safe} = engine; delete safe.error;
    lastReport = {platform: data.platform, version: data.version, engine: safe, gateway: {stage: g.stage, model: g.model, loadSeconds: g.loadSeconds}, clients: data.clientReports};
    $('health').textContent = JSON.stringify(lastReport, null, 2); $('log').textContent = log || 'No engine session yet.';
    renderDownload(); controls(); updateClientCards();
    if (!prefsLoaded) await loadLaunchPreferences();
    if (first) {
      $('status').textContent = 'Checking your model library…';
      $('detail').textContent = 'Reading model information from the drive. Your apps will be ready shortly.';
      if (!await refresh()) throw Error('Model discovery did not finish. Retrying automatically.');
      // Recommendations are optional. A failed recommendation must not disable
      // app launch after the model inventory has loaded successfully.
      try {
        const rec = await api('recommend?purpose=coding');
        $('recommendation').textContent = rec.reason + (rec.model ? ' Suggested: ' + rec.model + ' · ' + rec.context + ' context' : '');
        if (!engine.active && (engine.savedSettings || engine.settings).host !== hostKey() && rec.model) {
          selected = rec.model; $('context').value = rec.context; $('backend').value = rec.backend;
          if (!await refresh()) throw Error('Could not refresh models for the recommended settings.');
          $('recommendation').textContent = 'Starting settings adapted to this computer. ' + rec.reason;
        }
      } catch (e) {
        console.error('Reflect Go settings recommendation failed', e);
        $('recommendation').textContent = 'Automatic recommendation is unavailable. Current settings are retained; use Recommend settings to retry.';
      }
      initialized = true; controls();
    } else if (!wasLocked && (engine.active || engine.preparing) && !launching && !serviceStarting) {
      setSettings(engine.savedSettings || engine.settings);
      await refresh();
    }
    const signature = engine.ready ? JSON.stringify(engine.settings) : '';
    if (signature && signature !== providerSignature) { providerSignature = signature; await loadProviderModels(); }
    if (!signature) providerSignature = '';
  } catch (e) {
    console.error('Reflect Go status update failed', e);
    $('connection').hidden = false; $('connection').textContent = 'Could not refresh Reflect Go: ' + (e.message || String(e)) + ' Retrying…';
  }
  finally { if (!closed) setTimeout(poll, 2000); }
}
function renderIntegrations() {
  const container = $('integrations'), unavailable = $('unavailable-integrations');
  container.replaceChildren(); unavailable.replaceChildren();
  const ready = integrations.filter(app => app.available || app.connected);
  const otherCount = integrations.length - ready.length, connected = integrations.filter(app => app.connected).length;
  $('integration-summary').textContent = connected ? connected + ' connected' : ready.length ? ready.length + ' available' : '';
  $('unavailable-apps').hidden = !otherCount;
  $('unavailable-count').textContent = '(' + otherCount + ')';
  // Recovery controls for a saved desktop connection must remain discoverable.
  if (!integrationsSeen && connected) $('more-apps').open = true;
  integrationsSeen = true;
  for (const app of integrations) {
    const card = document.createElement('article'); card.className = 'integration-card';
    const header = document.createElement('div'); header.className = 'cardhead';
    const name = document.createElement('h3'); name.textContent = app.name;
    const kind = document.createElement('span'); kind.className = 'pill'; kind.textContent = app.kind || 'INTEGRATION'; header.append(name, kind);
    const detail = document.createElement('p'); detail.textContent = app.detail;
    const actions = document.createElement('div'); actions.className = 'integration-actions';
    const open = document.createElement('button'); open.textContent = app.available ? (app.actionLabel || (app.connected ? 'Open app' : 'Connect & open')) : 'Not available';
    open.dataset.integrationAction = 'launch'; open.dataset.available = app.available ? 'yes' : ''; open.disabled = !app.available || !!launching;
    open.onclick = () => launchIntegration(app, 'launch'); actions.append(open);
    if (app.connected) {
      const restore = document.createElement('button'); restore.textContent = 'Restore previous settings';
      restore.dataset.integrationAction = 'disconnect'; restore.dataset.available = 'yes'; restore.disabled = !!launching;
      restore.onclick = () => launchIntegration(app, 'disconnect'); actions.append(restore);
    }
    card.append(header, detail, actions);
    (app.available || app.connected ? container : unavailable).append(card);
  }
  if (!ready.length) container.textContent = 'No additional apps are ready on this computer. Check the integrations below for requirements.';
}
async function loadIntegrations() {
  integrationsLoading = true; controls();
  try { integrations = (await api('integrations')).integrations || []; renderIntegrations(); }
  catch (e) { $('integrations').textContent = 'Could not check additional apps. Use Check apps to retry.'; }
  finally { integrationsLoading = false; controls(); }
}
async function launchIntegration(app, action) {
  if (launching || serviceStarting) return;
  clearError(); launching = app.id; controls(); renderModels();
  $('integration-result').textContent = action === 'disconnect' ? 'Restoring settings for ' + app.name + '…' : 'Preparing your local models for ' + app.name + '…';
  try {
    await savePreferences(); await runSave.catch(() => {});
    const request = {id: app.id, project: $('project').value.trim(), settings: settings(), confirmRestart: false, confirmEngineRestart: false};
    async function perform() {
      const response = await fetch('/api/integrations/' + action, {method: 'POST', headers: {Authorization: 'Bearer ' + token, 'Content-Type': 'application/json'}, body: JSON.stringify(request)});
      const text = await response.text(); let result;
      try { result = JSON.parse(text); } catch { result = {message: text}; }
      if (response.status === 409 && result.needsEngineRestart && !request.confirmEngineRestart) {
        if (!confirm(result.message + '\n\nRestart the local service now?')) return {message: 'The service was not restarted.'};
        request.confirmEngineRestart = true; return perform();
      }
      if (response.status === 409 && result.needsRestart && !request.confirmRestart) {
        if (!confirm(result.message + '\n\nRestart this app now?')) return {message: 'No app was restarted.'};
        request.confirmRestart = true; return perform();
      }
      if (!response.ok) throw Error(result.message || result.error || text);
      return result;
    }
    const result = await perform(); if (action === 'launch') await syncEngineSettings(); $('integration-result').textContent = result.message || (action === 'disconnect' ? 'Previous settings restored.' : app.name + ' requested.');
    await loadIntegrations();
  } catch (e) { $('integration-result').textContent = app.name + ' connection did not finish.'; error(e); }
  finally { launching = ''; controls(); renderModels(); }
}
$('refresh-integrations').onclick = loadIntegrations;
function renderCatalog(){const container=$('catalog');container.replaceChildren();let visible=0;for(const m of catalog){const installed=inv?.models.some(i=>i.name===m.filename);const estimate=m.size*1.3+1073741824+2048*524288;const fit=inv&&estimate<=inv.hardware.screeningBudgetBytes;if($('catalog-purpose').value!=='all'&&$('catalog-purpose').value!==m.purpose)continue;if($('catalog-installed').value==='installed'&&!installed||$('catalog-installed').value==='download'&&installed)continue;if($('catalog-fit').checked&&!fit)continue;if(m.size>Number($('catalog-size').value)*1073741824)continue;visible++;const row=document.createElement('div');row.className='model-row';const title=document.createElement('h3');title.textContent=m.name+' · '+m.quantization;const description=document.createElement('p');description.textContent=gib(m.size)+' download · '+m.license+' · '+m.note;const screen=document.createElement('p');screen.textContent='Conservative estimate at 2,048 context: '+gib(estimate)+(fit?' — within budget':' — above budget or hardware not yet detected');const source=document.createElement('a');source.textContent='Model source and license';source.href=m.source;source.target='_blank';source.rel='noopener noreferrer';const button=document.createElement('button');button.textContent=installed?'Installed':'Download / resume';button.disabled=installed||!!download?.active;button.onclick=async()=>{clearError();try{download=await api('download',{id:m.id});renderCatalog();renderDownload();}catch(e){error(e)}};row.append(title,description,screen,source,button);container.append(row);}$('filter-summary').textContent=visible+' of '+catalog.length+' catalog models shown. Clear filters to see excluded choices.';}
let lastDownloadStage='';
function renderDownload(){if(!download?.id)return;$('download-status').textContent=download.stage+' · '+gib(download.received)+' / '+gib(download.total)+(download.error?' — '+download.error:'');$('download-progress').hidden=!download.active;$('download-progress').max=download.total||1;$('download-progress').value=download.received;$('cancel-download').hidden=!download.active;if(download.stage!==lastDownloadStage){const previous=lastDownloadStage;lastDownloadStage=download.stage;renderCatalog();if(download.stage==='complete'&&previous)refresh();}}
$('refresh').onclick = () => { clearError(); refresh(); };
for (const key of ['context', 'backend']) $(key).onchange = async () => { await refresh(); saveRunSettings().catch(() => {}); };
for (const key of ['temperature', 'threads', 'port']) $(key).onchange = () => { saveRunSettings().catch(() => {}); };
$('recommend').onclick = async () => {
  clearError();
  try {
    const r = await api('recommend?purpose=' + $('purpose').value); $('recommendation').textContent = r.reason;
    if (r.model) { selected = r.model; $('context').value = r.context; $('backend').value = r.backend; await refresh(); await saveRunSettings(); }
  } catch (e) { error(e); }
};
async function startService() {
  if (serviceStarting || launching) return;
  clearError(); serviceStarting = true; controls();
  try {
    if (!sharedModels().length) throw Error('Choose a suitable model in Models → Your models, or add one in Models → Add models.');
    await runSave.catch(() => {}); await api('start', settings()); await syncEngineSettings(); renderModels();
  } catch (e) { error(e); } finally { serviceStarting = false; controls(); }
}
$('start').onclick = startService; $('chat-start').onclick = startService;
$('stop').onclick = async () => { if (!confirm('Stop the local service and unload its model? Finish or cancel client tasks first.')) return; chatController?.abort(); try { await api('stop', {}); } catch (e) { error(e); } };
$('test').onclick = async () => { clearError(); $('test').disabled = true; $('response').hidden = false; $('response').textContent = 'Loading if needed, then generating…'; try { const value = await api('test', {}); $('response').textContent = value.response; } catch (e) { $('response').textContent = ''; error(e); } finally { controls(); } };
for (const client of Object.keys(clientNames)) $(client).onclick = async () => {
  if (launching || serviceStarting) return;
  clearError(); launching = client; controls(); renderModels();
  $('client-result').textContent = engine?.ready ? 'Opening ' + clientNames[client] + ' with your saved models…' : 'Starting your local service, then opening ' + clientNames[client] + '…';
  try {
    await savePreferences(); await runSave.catch(() => {});
    await api('client', {client, project: $('project').value.trim(), task: $('task').value, settings: settings()});
    await syncEngineSettings();
    $('client-result').textContent = clientNames[client] + ' terminal requested. Choose a model there and get to work. If launch fails, the terminal and Diagnostics explain why.';
  } catch (e) { $('client-result').textContent = clientNames[client] + ' could not be opened.'; error(e); }
  finally { launching = ''; controls(); renderModels(); }
};
$('model-client').onchange = renderModels;
$('all-suitable-models').onchange = () => { enabled = $('all-suitable-models').checked ? null : new Set(sharedModels().map(m => m.id)); modelsChanged(true); };
$('inherit-models').onchange = () => {
  const client = $('model-client').value, pref = clientPreference(client);
  launchPrefs.clients[client] = {...pref, enabled: $('inherit-models').checked ? null : sharedModels().map(m => m.id)};
  modelsChanged(false);
};
$('client-default').onchange = () => {
  const client = $('model-client').value;
  launchPrefs.clients[client] = {...clientPreference(client), defaultModel: $('client-default').value}; modelsChanged(false);
};
$('automatic-default').onclick = () => { selected = ''; modelsChanged(true); };
document.querySelectorAll('[data-configure-client]').forEach(button => button.onclick = () => {
  openModelSettings(button.dataset.configureClient);
});
function updateProjectSummary() {
  const path = $('project').value.trim();
  const name = path.replace(/[\\/]+$/, '').split(/[\\/]/).pop();
  $('project-summary').textContent = path ? name || path : 'Portable Workspace';
  $('project-summary').title = path || 'The Workspace folder on your drive';
}
function projectChanged() { controls(); $('project-warning').hidden = true; savePreferences().catch(() => {}); }
$('project').onchange = projectChanged;
$('project').oninput = controls;
$('clear-project').onclick = () => { $('project').value = ''; projectChanged(); };
const folderPicker = window.webkit?.messageHandlers?.chooseProject;
if (folderPicker) { $('browse-project').hidden = false; $('project-help').textContent = 'Choose an existing project, or leave blank to start in your portable Workspace.'; }
$('browse-project').onclick = () => { if (!launching && !serviceStarting) folderPicker?.postMessage({}); };
window.reflectGoProjectSelected = path => { if (typeof path === 'string' && !launching) { $('project').value = path; projectChanged(); } };
$('cancel-download').onclick=async()=>{try{download=await api('download/cancel',{});}catch(e){error(e)}};
for(const id of ['catalog-purpose','catalog-installed','catalog-fit','catalog-size'])$(id).onchange=renderCatalog;
$('import-model').onclick=async()=>{const file=$('import-file').files[0];if(!file){error(Error('Choose a GGUF file first.'));return;}$('import-model').disabled=true;$('import-status').textContent='Copying and checking the model…';try{const r=await fetch('/api/import?name='+encodeURIComponent(file.name),{method:'POST',headers:{Authorization:'Bearer '+token},body:file});if(!r.ok)throw Error(await r.text());$('import-status').textContent='Imported. Restart the engine and reopen clients to refresh available models.';await refresh();}catch(e){$('import-status').textContent='Import did not finish.';error(e)}finally{$('import-model').disabled=false}};
function message(role,text){const entry=document.createElement('div');entry.className='chat-message '+role;const label=document.createElement('strong');label.textContent=role==='user'?'You':'Local model';const body=document.createElement('p');body.textContent=text;entry.append(label,body);$('messages').append(entry);return body;}
$('chat-model').onchange=()=>{$('chat-timing').textContent='The next response uses this model; conversation history is retained.';controls()};
$('chat-send').onclick=async()=>{const text=$('chat-input').value.trim();if(!text||chatController||chatStopping)return;clearError();messages.push({role:'user',content:text});message('user',text);$('chat-input').value='';const output=message('assistant','Loading model if needed…');chatController=new AbortController();$('chat-stop').hidden=false;controls();let reply='',pending='',first=0,finished=false;const started=performance.now();try{const response=await fetch('/api/chat',{method:'POST',headers:{Authorization:'Bearer '+token,'Content-Type':'application/json'},body:JSON.stringify({model:$('chat-model').value,messages}),signal:chatController.signal});if(!response.ok)throw Error(await response.text());const reader=response.body.getReader(),decoder=new TextDecoder();chatReader=reader;while(true){const part=await reader.read();if(part.done)break;pending+=decoder.decode(part.value,{stream:true});const lines=pending.split('\n');pending=lines.pop();for(const line of lines){if(!line.startsWith('data:'))continue;const data=line.slice(5).trim();if(data==='[DONE]'){finished=true;continue;}if(!data)continue;let event;try{event=JSON.parse(data)}catch{continue}if(event.error)throw Error(event.error.message||'Model response failed');const delta=event.choices?.[0]?.delta?.content;if(delta){if(!first){first=performance.now();$('chat-timing').textContent='First response: '+((first-started)/1000).toFixed(2)+' s';}reply+=delta;output.textContent=reply;}}}if(!finished)throw Error('The response stream ended early. Retry or check Diagnostics.');if(!reply)output.textContent='The model returned no text.';}catch(e){if(e.name==='AbortError'||chatController?.signal.aborted){output.textContent=(reply||'')+'\n[Response stopped]';}else{output.textContent=(reply||'')+'\n[Response failed: '+e.message+']';error(e)}}finally{if(reply)messages.push({role:'assistant',content:reply});else messages.pop();chatController=null;chatReader=null;$('chat-stop').hidden=true;controls();}};
async function stopChat(){if(!chatController)return;chatStopping=true;chatController.abort();chatReader?.cancel().catch(()=>{});try{await api('chat/cancel',{})}catch(e){error(e)}finally{chatStopping=false;controls()}}
$('chat-stop').onclick=stopChat;$('new-chat').onclick=()=>{messages=[];$('messages').replaceChildren();$('chat-timing').textContent='';};
$('theme').onchange=async()=>{applyTheme($('theme').value);try{await api('preferences',{theme:$('theme').value})}catch(e){error(e)}};
$('quit').onclick=async()=>{if((engine?.active||download?.active||integrations.some(app=>app.connected))&&!confirm('Finish client tasks first. Quit will restore connected desktop apps to their previous settings and restart them, stop the engine, and pause downloads. Continue?'))return;chatController?.abort();try{await api('download/cancel',{});await api('quit',{});closed=true;sessionStorage.removeItem(storeKey);$('connection').hidden=false;$('connection').textContent='Workstation closed. Eject after the launcher exits.';document.querySelectorAll('button,input,select,textarea').forEach(el=>el.disabled=true);}catch(e){error(e)}};
$('copy').onclick=async()=>{try{await navigator.clipboard.writeText(JSON.stringify(lastReport,null,2));$('copy').textContent='Copied'}catch{error(Error('Clipboard access unavailable. Select and copy the report below.'))}};
const pageCopy = {
  workstation: ['Your AI. Anywhere.', 'Open an app. Your models go with you.'],
  library: ['Your models', 'Choose what travels with you and which apps can use it.'],
  chat: ['Chat', 'A simple conversation with a model on your drive.'],
  'settings-page': ['Settings', 'Appearance, performance, and connection details.'],
  guide: ['Plug in. Get going.', 'A few things to know before you start.']
};
function showPage(id) {
  if (!pageCopy[id]) return;
  document.querySelectorAll('.nav').forEach(button => {
    const current = button.dataset.page === id;
    button.classList.toggle('selected', current);
    if (current) button.setAttribute('aria-current', 'page'); else button.removeAttribute('aria-current');
  });
  document.querySelectorAll('.page').forEach(page => page.hidden = page.id !== id);
  [$('heading').textContent, $('page-description').textContent] = pageCopy[id];
  window.scrollTo(0, 0);
  $('heading').focus({preventScroll: true});
}
function showModelTab(id) {
  for (const panel of ['model-selection', 'model-downloads']) $(panel).hidden = panel !== id;
  document.querySelectorAll('[data-model-tab]').forEach(button => {
    const current = button.dataset.modelTab === id;
    button.classList.toggle('selected', current);
    button.setAttribute('aria-pressed', String(current));
  });
}
function openModelSettings(client = 'all') {
  showPage('library'); showModelTab('model-selection');
  $('model-client').value = client; $('model-settings').open = true; renderModels();
}
document.querySelectorAll('.nav').forEach(button => button.onclick = () => showPage(button.dataset.page));
document.querySelectorAll('[data-open-page]').forEach(button => button.onclick = () => showPage(button.dataset.openPage));
document.querySelectorAll('[data-model-tab]').forEach(button => button.onclick = () => showModelTab(button.dataset.modelTab));
$('manage-models').onclick = () => openModelSettings();
$('go-library').onclick = () => { showPage('library'); showModelTab('model-downloads'); };
(async () => {
  const preparation = await Promise.allSettled([
    (async () => { const p = await api('preferences'); $('theme').value = p.theme; applyTheme(p.theme); })(),
    (async () => { catalog = (await api('catalog')).models || []; renderCatalog(); })(),
    loadLaunchPreferences()
  ]);
  for (const result of preparation) if (result.status === 'rejected') error(result.reason);
  poll(); loadIntegrations();
})();
