const $ = (id) => document.getElementById(id);

function renderStatus(status) {
  $('profile-status').textContent = status.profile_status === 'traffic_seen' ? 'traffic seen' : 'generated';
  $('identity-status').textContent = status.identity_enrolled ? 'seen' : 'waiting';
  $('stage2-status').textContent = status.stage2_delivered ? 'seen' : 'waiting';
  $('doh-status').textContent = status.doh_seen ? 'seen' : 'waiting';
  $('proxy-status').textContent = status.proxy_seen ? 'seen' : 'waiting';
  $('revision').textContent = String(status.location_revision ?? 0);
  if (status.selected_label) $('label').value = status.selected_label;
  if (status.selected_latitude !== undefined) $('latitude').value = status.selected_latitude;
  if (status.selected_longitude !== undefined) $('longitude').value = status.selected_longitude;
}

async function refreshStatus() {
  const response = await fetch('/api/status', { cache: 'no-store' });
  if (!response.ok) throw new Error(`status ${response.status}`);
  renderStatus(await response.json());
}

const labURL = `https://device-loc.${window.location.hostname}/v1/location`;
$('lab-url').href = labURL;
$('lab-url').textContent = labURL;

$('location-form').addEventListener('submit', async (event) => {
  event.preventDefault();
  const message = $('save-message');
  message.textContent = 'Saving…';
  try {
    const response = await fetch('/api/location', {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({
        latitude: Number($('latitude').value),
        longitude: Number($('longitude').value),
        label: $('label').value,
      }),
    });
    if (!response.ok) throw new Error(await response.text());
    renderStatus(await response.json());
    message.textContent = 'Target saved. Open the controlled lab endpoint to verify this revision.';
  } catch (error) {
    message.textContent = `Could not save target: ${String(error.message || error)}`;
  }
});

refreshStatus().catch((error) => {
  $('save-message').textContent = `Could not load status: ${String(error.message || error)}`;
});
setInterval(() => refreshStatus().catch(() => {}), 5000);


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
    message.textContent = 'Enrollment reset. Remove the old iPhone profile, then download and install the new Stage 1 profile.';
  } catch (error) {
    message.textContent = `Could not reset enrollment: ${String(error.message || error)}`;
  }
});
