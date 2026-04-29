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

document.addEventListener('alpine:init', () => {
  const appData = getAppData();

  Alpine.data('fanController', () => ({
    fans: structuredClone(appData.fans || initialFans).map((fan) => ({
      ...fan,
      originalSpeed: fan.originalSpeed ?? fan.speed,
    })),
    temperatures: structuredClone(appData.temperatures || []),
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
    socketConnected: false,
    socket: null,
    newPresetModal: {
      open: false,
      name: '',
      error: '',
    },

    init() {
      this.applyTheme(this.theme);
      this.detectPreset();
      this.connectConsole();
    },

    applyTheme(theme) {
      this.theme = theme;
      document.documentElement.setAttribute('data-theme', theme);
      localStorage.setItem('ilo-theme', theme);
    },

    cycleTheme() {
      this.applyTheme(this.theme === 'night' ? 'light' : 'night');
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
        this.status = {
          type: 'error',
          message: error.message || 'Unable to apply fan speeds.',
        };
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

    getPendingCommands() {
      return this.fans.flatMap((fan, index) => {
        if (fan.speed === fan.originalSpeed) {
          return [];
        }

        const fanIndex = Number.isInteger(fan.commandNumber) ? fan.commandNumber : index + 1;
        return [
          `fan p ${fanIndex} max ${percentageToILOValue(fan.speed)}`,
          `fan p ${fanIndex} min 255`,
        ];
      });
    },
  }));
});

Alpine.start();
