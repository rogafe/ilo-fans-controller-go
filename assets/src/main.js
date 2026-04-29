import 'htmx.org';
import Alpine from 'alpinejs';
import 'flowbite';

window.Alpine = Alpine;

const initialFans = [
  { name: 'Fan 1', speed: 85, originalSpeed: 85 },
  { name: 'Fan 2', speed: 48, originalSpeed: 48 },
  { name: 'Fan 3', speed: 69, originalSpeed: 69 },
  { name: 'Fan 4', speed: 18, originalSpeed: 18 },
  { name: 'Fan 5', speed: 44, originalSpeed: 44 },
  { name: 'Fan 6', speed: 96, originalSpeed: 96 },
];

const initialPresets = [
  { name: 'Silent Mode', speeds: [15] },
  { name: 'Normal Mode', speeds: [50] },
  { name: 'Turbo Mode', speeds: [100] },
];

function clampSpeed(speed) {
  if (Number.isNaN(speed)) {
    return 15;
  }

  return Math.min(100, Math.max(15, speed));
}

function getAppData() {
  return window.__APP_DATA__ || {
    fans: initialFans,
    temperatures: [],
    presets: initialPresets,
    minimumFanSpeed: 15,
    status: null,
  };
}

function getClientId() {
  const existingClientId = window.sessionStorage.getItem('ilo-console-client-id');
  if (existingClientId) {
    return existingClientId;
  }

  const clientId = window.crypto?.randomUUID?.() || `client-${Date.now()}`;
  window.sessionStorage.setItem('ilo-console-client-id', clientId);
  return clientId;
}

function percentageToILOValue(speed) {
  return Math.ceil((Number(speed) / 100) * 255);
}

function getDefaultConsoleOpen() {
  try {
    const stored = window.localStorage.getItem('ilo-console-open');
    if (stored === '1') return true;
    if (stored === '0') return false;
  } catch {
    // ignore
  }

  return window.matchMedia?.('(min-width: 1024px)')?.matches ?? true;
}

function getCollapsedTempGroups() {
  try {
    const raw = window.localStorage.getItem('ilo-temp-collapsed-groups');
    if (!raw) return {};
    const parsed = JSON.parse(raw);
    return parsed && typeof parsed === 'object' && !Array.isArray(parsed) ? parsed : {};
  } catch {
    return {};
  }
}

function persistCollapsedTempGroups(value) {
  try {
    window.localStorage.setItem('ilo-temp-collapsed-groups', JSON.stringify(value || {}));
  } catch {
    // ignore
  }
}

document.addEventListener('alpine:init', () => {
  const appData = getAppData();

  Alpine.data('fanController', () => ({
    fans: structuredClone(appData.fans || initialFans).map((fan) => ({
      ...fan,
      originalSpeed: fan.originalSpeed ?? fan.speed,
    })),
    temperatures: structuredClone(appData.temperatures || []),
    temperatureFilter: 'all',
    collapsedTempGroups: getCollapsedTempGroups(),
    presets: structuredClone(appData.presets || initialPresets),
    currentPreset: null,
    editAll: false,
    isLoading: false,
    requestTime: null,
    theme: localStorage.getItem('ilo-theme') || 'night',
    minimumFanSpeed: appData.minimumFanSpeed || 15,
    status: appData.status,
    clientId: getClientId(),
    consoleLines: [],
    consoleOpen: getDefaultConsoleOpen(),
    socketConnected: false,
    socket: null,
    offlineRetryTimer: null,
    newPresetModal: {
      open: false,
      name: '',
      error: '',
    },

    init() {
      this.applyTheme(this.theme);
      this.detectPreset();
      this.connectConsole();
      if (this.status?.type === 'offline') {
        this.startOfflineRetry();
      }
    },

    setOffline(message) {
      this.status = {
        type: 'offline',
        message: message || 'iLO is unreachable. Waiting for it to come back...',
      };
      this.startOfflineRetry();
    },

    startOfflineRetry() {
      if (this.offlineRetryTimer) {
        return;
      }

      const tick = async () => {
        try {
          const response = await fetch('/api/fans', { method: 'GET' });
          if (!response.ok) {
            if (response.status === 503) {
              const payload = await response.json().catch(() => ({}));
              this.status = { type: 'offline', message: payload.error || this.status?.message };
            }
            return;
          }

          const updatedFans = await response.json();
          this.fans = (updatedFans || []).map((fan) => ({
            ...fan,
            originalSpeed: fan.speed,
          }));
          this.detectPreset();
          if (this.status?.type === 'offline') {
            this.status = { type: 'success', message: 'iLO is reachable again.' };
          }
          this.stopOfflineRetry();
        } catch {
          // ignore and keep waiting
        }
      };

      this.offlineRetryTimer = window.setInterval(tick, 5000);
      tick();
    },

    stopOfflineRetry() {
      if (!this.offlineRetryTimer) return;
      window.clearInterval(this.offlineRetryTimer);
      this.offlineRetryTimer = null;
    },

    applyTheme(theme) {
      this.theme = theme;
      document.documentElement.setAttribute('data-theme', theme);
      localStorage.setItem('ilo-theme', theme);
    },

    cycleTheme() {
      this.applyTheme(this.theme === 'night' ? 'light' : 'night');
    },

    toggleConsole() {
      this.consoleOpen = !this.consoleOpen;
      try {
        window.localStorage.setItem('ilo-console-open', this.consoleOpen ? '1' : '0');
      } catch {
        // ignore
      }
    },

    clearConsole() {
      this.consoleLines = [];
    },

    setTemperatureFilter(value) {
      this.temperatureFilter = value || 'all';
    },

    filteredTemperatures() {
      const items = Array.isArray(this.temperatures) ? this.temperatures : [];

      if (this.temperatureFilter === 'all') {
        return items;
      }

      const filterTone = this.temperatureFilter;
      return items.filter((temperature) => this.temperatureTone(temperature) === filterTone);
    },

    temperatureSummary() {
      const summary = {
        total: 0,
        ok: 0,
        warning: 0,
        error: 0,
        absent: 0,
        maxTemp: null,
        maxTone: 'success',
      };

      const items = Array.isArray(this.temperatures) ? this.temperatures : [];
      summary.total = items.length;

      for (const temperature of items) {
        const tone = this.temperatureTone(temperature);

        if (tone === 'ghost') summary.absent += 1;
        else if (tone === 'error') summary.error += 1;
        else if (tone === 'warning') summary.warning += 1;
        else summary.ok += 1;

        if (typeof temperature?.temperature === 'number') {
          if (summary.maxTemp === null || temperature.temperature > summary.maxTemp) {
            summary.maxTemp = temperature.temperature;
            summary.maxTone = tone === 'ghost' ? 'success' : tone;
          }
        }
      }

      return summary;
    },

    temperatureGroupKey(temperature) {
      const physicalContext = this.humanizeTemperatureToken(temperature?.physicalContext);
      const localeLabel = temperature?.localeLabel?.trim();
      const key = (physicalContext || localeLabel || 'Other').trim();
      return key || 'Other';
    },

    groupedTemperatures() {
      const items = this.filteredTemperatures();
      const groups = new Map();

      for (const temperature of items) {
        const key = this.temperatureGroupKey(temperature);
        if (!groups.has(key)) {
          groups.set(key, {
            key,
            label: key,
            sensors: [],
            counts: { ok: 0, warning: 0, error: 0, absent: 0, total: 0 },
            maxTemp: null,
            maxTone: 'success',
            hasNonOk: false,
          });
        }

        const group = groups.get(key);
        group.sensors.push(temperature);
        group.counts.total += 1;

        const tone = this.temperatureTone(temperature);
        if (tone === 'ghost') group.counts.absent += 1;
        else if (tone === 'error') group.counts.error += 1;
        else if (tone === 'warning') group.counts.warning += 1;
        else group.counts.ok += 1;

        if (tone === 'error' || tone === 'warning') {
          group.hasNonOk = true;
        }

        if (typeof temperature?.temperature === 'number') {
          if (group.maxTemp === null || temperature.temperature > group.maxTemp) {
            group.maxTemp = temperature.temperature;
            group.maxTone = tone === 'ghost' ? 'success' : tone;
          }
        }
      }

      const result = Array.from(groups.values()).map((group) => ({
        ...group,
        sensors: group.sensors.sort((a, b) => (a?.index ?? 0) - (b?.index ?? 0)),
      }));

      result.sort((a, b) => a.label.localeCompare(b.label));
      return result;
    },

    isTempGroupCollapsed(key) {
      return Boolean(this.collapsedTempGroups?.[key]);
    },

    isTempGroupOpen(group) {
      if (!group?.key) return true;
      if (this.isTempGroupCollapsed(group.key)) return false;
      return Boolean(group.hasNonOk);
    },

    onTempGroupToggle(key, isOpen) {
      if (!key) return;
      const next = { ...(this.collapsedTempGroups || {}) };

      if (isOpen) {
        delete next[key];
      } else {
        next[key] = true;
      }

      this.collapsedTempGroups = next;
      persistCollapsedTempGroups(next);
    },

    updateFanSpeed(index, speed) {
      const nextSpeed = Math.min(100, Math.max(this.minimumFanSpeed, Number(clampSpeed(Number(speed)))));

      if (this.editAll) {
        this.fans = this.fans.map((fan) => ({ ...fan, speed: nextSpeed }));
      } else {
        this.fans[index].speed = nextSpeed;
      }

      this.detectPreset();
    },

    connectConsole() {
      const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
      const socketUrl = `${protocol}://${window.location.host}/ws/console?client_id=${encodeURIComponent(this.clientId)}`;
      this.socket = new WebSocket(socketUrl);

      this.socket.addEventListener('open', () => {
        this.socketConnected = true;
      });

      this.socket.addEventListener('message', (event) => {
        try {
          const payload = JSON.parse(event.data);
          this.consoleLines.push(payload);
        } catch {
          this.consoleLines.push({
            type: 'info',
            message: event.data,
            timestamp: new Date().toISOString(),
          });
        }
      });

      this.socket.addEventListener('close', () => {
        this.socketConnected = false;
      });

      this.socket.addEventListener('error', () => {
        this.socketConnected = false;
      });
    },

    resetFan(index) {
      const originalSpeed = this.fans[index].originalSpeed;

      if (this.editAll) {
        this.fans = this.fans.map((fan) => ({ ...fan, speed: originalSpeed }));
      } else {
        this.fans[index].speed = originalSpeed;
      }

      this.detectPreset();
    },

    applyPreset(index) {
      const preset = this.presets[index];
      this.fans = this.fans.map((fan, fanIndex) => ({
        ...fan,
        speed: preset.speeds.length === 1 ? preset.speeds[0] : preset.speeds[fanIndex],
      }));
      this.currentPreset = index;
    },

    openNewPresetModal() {
      this.newPresetModal.name = '';
      this.newPresetModal.error = '';
      this.newPresetModal.open = true;
      this.$nextTick(() => {
        this.$refs.presetNameInput?.focus();
      });
    },

    closeNewPresetModal() {
      this.newPresetModal.open = false;
      this.newPresetModal.name = '';
      this.newPresetModal.error = '';
    },

    confirmNewPreset() {
      const name = this.newPresetModal.name.trim();
      if (!name) {
        this.newPresetModal.error = 'Please enter a preset name.';
        return;
      }

      const speeds = this.fans.map((fan) => fan.speed);
      const existingPreset = this.presets.find((preset) => {
        const normalized = preset.speeds.length === 1
          ? this.fans.map(() => preset.speeds[0])
          : preset.speeds;

        return normalized.join(',') === speeds.join(',');
      });

      if (existingPreset) {
        this.newPresetModal.error = `A preset with these speeds already exists: "${existingPreset.name}".`;
        return;
      }

      this.presets.push({ name, speeds });
      this.currentPreset = this.presets.length - 1;
      this.closeNewPresetModal();
      this.savePresets();
    },

    async savePresets() {
      try {
        const response = await fetch('/api/presets', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
          },
          body: JSON.stringify(this.presets),
        });

        if (!response.ok) {
          const payload = await response.json().catch(() => ({}));
          throw new Error(payload.error || 'Unable to save presets');
        }

        this.presets = await response.json();
        this.status = {
          type: 'success',
          message: 'Presets saved.',
        };
        this.detectPreset();
      } catch (error) {
        this.status = {
          type: 'error',
          message: error.message || 'Unable to save presets.',
        };
      }
    },

    detectPreset() {
      const speeds = this.fans.map((fan) => fan.speed);

      const matchedPresetIndex = this.presets.findIndex((preset) => {
        const normalized = preset.speeds.length === 1
          ? this.fans.map(() => preset.speeds[0])
          : preset.speeds;

        return normalized.length === speeds.length && normalized.every((speed, index) => speed === speeds[index]);
      });

      this.currentPreset = matchedPresetIndex === -1 ? null : matchedPresetIndex;
    },

    async applySpeeds() {
      if (this.status?.type === 'offline') {
        this.setOffline(this.status?.message);
        return;
      }

      this.isLoading = true;
      this.requestTime = null;
      const startTime = performance.now();
      this.consoleLines = [];

      try {
        const fanPayload = Object.fromEntries(this.fans.map((fan) => [fan.name, fan.speed]));
        const response = await fetch('/api/fans', {
          method: 'POST',
          headers: {
            'Content-Type': 'application/json',
            'X-Console-Client-Id': this.clientId,
          },
          body: JSON.stringify({ clientId: this.clientId, fans: fanPayload }),
        });

        if (!response.ok) {
          const payload = await response.json().catch(() => ({}));
          if (response.status === 503) {
            this.setOffline(payload.error || 'iLO is unreachable (server completely off). Waiting...');
            throw new Error(payload.error || 'iLO is unreachable');
          }
          throw new Error(payload.error || 'Unable to apply fan speeds');
        }

        const updatedFans = await response.json();
        this.fans = updatedFans.map((fan) => ({
          ...fan,
          originalSpeed: fan.speed,
        }));
        this.requestTime = Math.round(performance.now() - startTime);
        this.status = {
          type: 'success',
          message: 'Fan speeds applied successfully.',
        };
        this.detectPreset();
      } catch (error) {
        if (this.status?.type !== 'offline') {
          this.status = {
            type: 'error',
            message: error.message || 'Unable to apply fan speeds.',
          };
        }
      } finally {
        this.isLoading = false;
      }
    },

    formatRequestTime() {
      if (this.requestTime === null) {
        return '';
      }

      return this.requestTime >= 1000
        ? `${(this.requestTime / 1000).toFixed(2)}s`
        : `${this.requestTime}ms`;
    },

    formatConsoleTimestamp(timestamp) {
      if (!timestamp) {
        return '--:--:--';
      }

      return new Date(timestamp).toLocaleTimeString();
    },

    formatTemperatureName(temperature) {
      const label = temperature?.label?.trim();
      return label || `Sensor ${String(temperature.index).padStart(2, '0')}`;
    },

    formatTemperatureLocation(temperature) {
      const physicalContext = this.humanizeTemperatureToken(temperature?.physicalContext);
      const localeLabel = temperature?.localeLabel?.trim();

      if (physicalContext && localeLabel && physicalContext !== localeLabel) {
        return `${physicalContext} · ${localeLabel}`;
      }

      return physicalContext || localeLabel || 'Unknown';
    },

    formatTemperatureThresholds(temperature) {
      const thresholds = [];

      if (temperature.cautionThreshold > 0) {
        thresholds.push(`Caution ${temperature.cautionThreshold}C`);
      }

      if (temperature.criticalThreshold > 0) {
        thresholds.push(`Critical ${temperature.criticalThreshold}C`);
      }

      if (thresholds.length > 0) {
        return thresholds.join(' / ');
      }

      if (temperature.threshold > 0) {
        const thresholdType = temperature.thresholdTypeLabel || 'Threshold';
        return `${thresholdType} ${temperature.threshold}C`;
      }

      return 'N/A';
    },

    formatTemperatureStatus(temperature) {
      if (temperature?.state === 'Absent') {
        return 'Absent';
      }

      return temperature?.health || temperature?.conditionLabel || 'Unknown';
    },

    hasTemperatureCoordinates(temperature) {
      return Boolean(temperature?.physicalContext) || temperature?.locationX !== 0 || temperature?.locationY !== 0;
    },

    temperatureTone(temperature) {
      const status = `${temperature?.health || ''} ${temperature?.conditionLabel || ''} ${temperature?.state || ''}`.toLowerCase();

      if (status.includes('absent')) {
        return 'ghost';
      }

      if (status.includes('critical') || status.includes('failed')) {
        return 'error';
      }

      if (status.includes('degraded') || status.includes('warning') || status.includes('caution')) {
        return 'warning';
      }

      return 'success';
    },

    humanizeTemperatureToken(value) {
      if (!value) {
        return '';
      }

      return String(value)
        .replace(/([a-z0-9])([A-Z])/g, '$1 $2')
        .replace(/[_-]+/g, ' ')
        .trim();
    },

    getPendingCommands() {
      return this.fans.flatMap((fan, index) => {
        if (fan.speed === fan.originalSpeed) {
          return [];
        }

        const fanIndex = Number.isInteger(fan.commandNumber) ? fan.commandNumber : index + 1;
        return [
          `fan p ${fanIndex} min ${percentageToILOValue(fan.speed)}`,
          `fan p ${fanIndex} max 255`,
        ];
      });
    },
  }));
});

Alpine.start();
