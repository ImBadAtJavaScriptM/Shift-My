const $ = (id) => document.getElementById(id);

const locationForm = $('location-form');
let locationFormDirty = false;
let currentStatus = null;
let latestHistory = [];

for (const id of ['label', 'latitude', 'longitude']) {
  $(id).addEventListener('input', () => {
    locationFormDirty = true;
  });
}

function locationFieldsAreBeingEdited() {
  return locationFormDirty || locationForm.contains(document.activeElement);
}

function formatCoords(lat, lon) {
  return `${Number(lat).toFixed(5)}, ${Number(lon).toFixed(5)}`;
}

function updateMap(lat, lon, label) {
  if (!Number.isFinite(Number(lat)) || !Number.isFinite(Number(lon))) return;
  const latitude = Number(lat);
  const longitude = Number(lon);
  const delta = 0.008;
  const bbox = [
    longitude - delta,
    latitude - delta,
    longitude + delta,
    latitude + delta,
  ].join(',');
  $('map-preview').src =
    `https://www.openstreetmap.org/export/embed.html?bbox=${encodeURIComponent(bbox)}&layer=mapnik&marker=${encodeURIComponent(latitude + ',' + longitude)}`;
  $('open-map').href =
    `https://www.openstreetmap.org/?mlat=${encodeURIComponent(latitude)}&mlon=${encodeURIComponent(longitude)}#map=16/${encodeURIComponent(latitude)}/${encodeURIComponent(longitude)}`;
  $('map-preview').title = label ? `${label} map` : 'Current target map';
}

function updateLastChanged(revision) {
  const item = latestHistory.find((entry) => Number(entry.revision) === Number(revision));
  if (!item) {
    $('current-target-time').textContent = `Revision ${revision ?? 0}`;
    return;
  }
  const when = new Date(item.created_at);
  $('current-target-time').textContent =
    `Revision ${item.revision} · changed ${when.toLocaleString()}`;
}

function setHealth(id, ok) {
  const el = $(id);
  el.textContent = ok ? 'seen' : 'waiting';
  el.closest('.status-card')?.classList.toggle('healthy', Boolean(ok));
}

function renderStatus(status, { forceLocation = false } = {}) {
  currentStatus = status;
  setHealth('identity-status', status.identity_enrolled);
  setHealth('stage2-status', status.stage2_delivered);
  setHealth('doh-status', status.doh_seen);
  setHealth('proxy-status', status.proxy_seen);
  $('revision').textContent = String(status.location_revision ?? 0);

  const hasTarget =
    status.selected_latitude !== undefined &&
    status.selected_longitude !== undefined;
  if (hasTarget) {
    $('current-target-label').textContent = status.selected_label || 'Unnamed target';
    $('current-target-coords').textContent =
      formatCoords(status.selected_latitude, status.selected_longitude);
    updateMap(status.selected_latitude, status.selected_longitude, status.selected_label);
  } else {
    $('current-target-label').textContent = 'No target selected';
    $('current-target-coords').textContent = '—';
  }
  updateLastChanged(status.location_revision);

  if (forceLocation || !locationFieldsAreBeingEdited()) {
    if (status.selected_label) $('label').value = status.selected_label;
    if (status.selected_latitude !== undefined) $('latitude').value = status.selected_latitude;
    if (status.selected_longitude !== undefined) $('longitude').value = status.selected_longitude;
  }
}

async function refreshStatus() {
  const response = await fetch('/api/status', { cache: 'no-store' });
  if (!response.ok) throw new Error(`status ${response.status}`);
  renderStatus(await response.json());
}

async function loadHistory() {
  const response = await fetch('/api/location/history', { cache: 'no-store' });
  if (!response.ok) throw new Error(`history ${response.status}`);
  latestHistory = await response.json();
  renderHistory(latestHistory);
  if (currentStatus) updateLastChanged(currentStatus.location_revision);
}

async function loadPresets() {
  const response = await fetch('/api/location/presets', { cache: 'no-store' });
  if (!response.ok) throw new Error(`presets ${response.status}`);
  renderPresets(await response.json());
}

function makeLocationRow(item, { deletable = false } = {}) {
  const row = document.createElement('div');
  row.className = 'location-row';

  const copy = document.createElement('div');
  copy.className = 'location-copy';

  const title = document.createElement('strong');
  title.textContent = item.label || 'Unnamed target';
  const meta = document.createElement('span');
  meta.textContent = formatCoords(item.latitude, item.longitude);
  copy.append(title, meta);

  const buttons = document.createElement('div');
  buttons.className = 'mini-actions';

  const use = document.createElement('button');
  use.type = 'button';
  use.className = 'mini-button';
  use.textContent = 'Switch';
  use.addEventListener('click', () => switchTarget(item));
  buttons.append(use);

  if (deletable) {
    const remove = document.createElement('button');
    remove.type = 'button';
    remove.className = 'mini-button danger-button';
    remove.textContent = '×';
    remove.setAttribute('aria-label', `Delete ${item.label || 'saved location'}`);
    remove.addEventListener('click', async () => {
      const response = await fetch(`/api/location/presets?id=${encodeURIComponent(item.id)}`, {
        method: 'DELETE',
      });
      if (!response.ok) {
        $('save-message').textContent = `Could not delete saved location: ${await response.text()}`;
        return;
      }
      await loadPresets();
    });
    buttons.append(remove);
  }

  row.append(copy, buttons);
  return row;
}

function renderHistory(items) {
  const host = $('history-list');
  host.replaceChildren();
  if (!items.length) {
    const empty = document.createElement('p');
    empty.className = 'subtle';
    empty.textContent = 'No switches recorded yet.';
    host.append(empty);
    return;
  }
  for (const item of items) {
    const row = makeLocationRow(item);
    const when = document.createElement('small');
    when.textContent = `rev ${item.revision} · ${new Date(item.created_at).toLocaleString()}`;
    row.querySelector('.location-copy').append(when);
    host.append(row);
  }
}

function renderPresets(items) {
  const host = $('preset-list');
  host.replaceChildren();
  if (!items.length) {
    const empty = document.createElement('p');
    empty.className = 'subtle';
    empty.textContent = 'Save a target to make it a one-tap location.';
    host.append(empty);
    return;
  }
  for (const item of items) host.append(makeLocationRow(item, { deletable: true }));
}

async function setTarget(latitude, longitude, label) {
  const response = await fetch('/api/location', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
      latitude: Number(latitude),
      longitude: Number(longitude),
      label: String(label || '').trim(),
    }),
  });
  if (!response.ok) throw new Error(await response.text());
  locationFormDirty = false;
  const status = await response.json();
  renderStatus(status, { forceLocation: true });
  await loadHistory();
  return status;
}

async function switchTarget(item) {
  const message = $('save-message');
  message.textContent = `Switching to ${item.label || 'target'}…`;
  try {
    await setTarget(item.latitude, item.longitude, item.label);
    message.textContent = `Target switched to ${item.label || 'selected location'}.`;
  } catch (error) {
    message.textContent = `Could not switch target: ${String(error.message || error)}`;
  }
}

const labURL = `https://device-loc.${window.location.hostname}/v1/location`;
$('lab-url').href = labURL;

locationForm.addEventListener('submit', async (event) => {
  event.preventDefault();
  const message = $('save-message');
  message.textContent = 'Saving…';
  try {
    await setTarget(
      $('latitude').value,
      $('longitude').value,
      $('label').value
    );
    message.textContent = 'Target saved. The controlled endpoint now uses this revision.';
  } catch (error) {
    message.textContent = `Could not save target: ${String(error.message || error)}`;
  }
});

$('save-preset').addEventListener('click', async () => {
  const message = $('save-message');
  const label = $('label').value.trim();
  const latitude = Number($('latitude').value);
  const longitude = Number($('longitude').value);
  if (!label || !Number.isFinite(latitude) || !Number.isFinite(longitude)) {
    message.textContent = 'Enter a label and valid coordinates before saving.';
    return;
  }
  message.textContent = 'Saving location…';
  try {
    const response = await fetch('/api/location/presets', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ label, latitude, longitude }),
    });
    if (!response.ok) throw new Error(await response.text());
    await loadPresets();
    message.textContent = `Saved ${label} for one-tap switching.`;
  } catch (error) {
    message.textContent = `Could not save location: ${String(error.message || error)}`;
  }
});

$('place-search-form').addEventListener('submit', async (event) => {
  event.preventDefault();
  const query = $('place-query').value.trim();
  const message = $('search-message');
  const host = $('search-results');
  if (!query) return;
  message.textContent = 'Searching OpenStreetMap…';
  host.replaceChildren();

  try {
    const url = new URL('https://nominatim.openstreetmap.org/search');
    url.searchParams.set('format', 'jsonv2');
    url.searchParams.set('limit', '5');
    url.searchParams.set('q', query);
    url.searchParams.set('addressdetails', '1');
    const response = await fetch(url.toString(), {
      headers: { Accept: 'application/json' },
    });
    if (!response.ok) throw new Error(`search ${response.status}`);
    const results = await response.json();
    if (!results.length) {
      message.textContent = 'No matching places found.';
      return;
    }
    message.textContent = 'Tap a result to load it into the target form.';
    for (const result of results) {
      const button = document.createElement('button');
      button.type = 'button';
      button.className = 'search-result';
      const title = document.createElement('strong');
      title.textContent = result.display_name;
      const meta = document.createElement('span');
      meta.textContent = formatCoords(result.lat, result.lon);
      button.append(title, meta);
      button.addEventListener('click', () => {
        const shortLabel =
          result.name ||
          result.display_name.split(',')[0] ||
          query;
        $('label').value = shortLabel;
        $('latitude').value = result.lat;
        $('longitude').value = result.lon;
        locationFormDirty = true;
        host.replaceChildren();
        message.textContent = `Loaded ${shortLabel}. Tap Set target to switch.`;
        locationForm.scrollIntoView({ behavior: 'smooth', block: 'center' });
      });
      host.append(button);
    }
  } catch (error) {
    message.textContent =
      `Place search unavailable. You can still enter coordinates manually. (${String(error.message || error)})`;
  }
});

$('reset-enrollment').addEventListener('click', async () => {
  const message = $('reset-message');
  const confirmed = window.confirm(
    'Reset this iPhone enrollment? This rotates profile/ACME credentials. Your saved target coordinates will stay unchanged.'
  );
  if (!confirmed) return;
  message.textContent = 'Resetting enrollment…';
  try {
    const response = await fetch('/api/enrollment/reset', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ confirm: 'RESET' }),
    });
    if (!response.ok) throw new Error(await response.text());
    renderStatus(await response.json());
    message.textContent =
      'Enrollment reset. Remove the old iPhone profile, then download and install the new Stage 1 profile.';
  } catch (error) {
    message.textContent = `Could not reset enrollment: ${String(error.message || error)}`;
  }
});

Promise.all([refreshStatus(), loadHistory(), loadPresets()]).catch((error) => {
  $('save-message').textContent = `Could not load dashboard: ${String(error.message || error)}`;
});

setInterval(() => refreshStatus().catch(() => {}), 5000);
