import { KeybindingItem } from '../../main/settings/settingsStore';

// Context types (minimal set for now)
export type ContextKey = 
  | 'editorFocus'
  | 'sidebarFocus'
  | 'inputFocus'
  | 'modalOpen'
  | 'always';

export type ContextState = {
  editorFocus?: boolean;
  sidebarFocus?: boolean;
  inputFocus?: boolean;
  modalOpen?: boolean;
};

// Command handler type
export type CommandHandler = (args?: unknown) => void | Promise<void>;

// Command registry
class CommandRegistry {
  private commands = new Map<string, CommandHandler>();

  register(command: string, handler: CommandHandler) {
    this.commands.set(command, handler);
  }

  unregister(command: string) {
    this.commands.delete(command);
  }

  execute(command: string, args?: unknown): void | Promise<void> {
    const handler = this.commands.get(command);
    if (handler) {
      return handler(args);
    }
    console.warn(`Command not found: ${command}`);
  }

  has(command: string): boolean {
    return this.commands.has(command);
  }
}

// Keybinding parser and matcher
class KeybindingMatcher {
  private keybindings: KeybindingItem[] = [];
  private contextState: ContextState = {};

  updateKeybindings(keybindings: KeybindingItem[]) {
    this.keybindings = keybindings;
  }

  updateContext(context: Partial<ContextState>) {
    this.contextState = { ...this.contextState, ...context };
  }

  private parseKeybinding(key: string): string[] {
    // Parse keybinding like "cmd+k cmd+s" into parts
    return key.split(/\s+/).filter(Boolean);
  }

  private normalizeKey(key: string): string {
    // Normalize key names (cmd -> meta, ctrl -> control, etc.)
    return key
      .toLowerCase()
      .replace(/cmd|meta|command/, 'meta')
      .replace(/ctrl|control/, 'control')
      .replace(/alt|option/, 'alt')
      .replace(/shift/, 'shift');
  }

  private matchesKeybinding(
    event: KeyboardEvent,
    keybindingParts: string[]
  ): boolean {
    // For now, we'll match the last part of the keybinding
    // Full chord support can be added later
    const lastPart = keybindingParts[keybindingParts.length - 1];
    const normalized = this.normalizeKey(lastPart);

    // Check modifiers
    const hasMeta = event.metaKey || event.ctrlKey; // Mac uses meta, others use ctrl
    const hasCtrl = event.ctrlKey;
    const hasAlt = event.altKey;
    const hasShift = event.shiftKey;

    // Extract the base key
    const baseKey = normalized.replace(/^(meta|control|alt|shift)\+/, '');
    const keyMatch = event.key.toLowerCase() === baseKey.toLowerCase() ||
                     event.code.toLowerCase().replace(/^key/, '') === baseKey.toLowerCase();

    if (!keyMatch) {
      return false;
    }

    // Check if modifiers match
    if (normalized.includes('meta') && !hasMeta) return false;
    if (normalized.includes('control') && !hasCtrl) return false;
    if (normalized.includes('alt') && !hasAlt) return false;
    if (normalized.includes('shift') && !hasShift) return false;

    return true;
  }

  private evaluateWhen(when?: string): boolean {
    if (!when) return true;

    // Simple context evaluation (can be extended later)
    const context = this.contextState;

    // Parse simple expressions like "editorFocus", "!modalOpen", "editorFocus && !inputFocus"
    const parts = when.split(/\s+(&&|\|\|)\s+/);
    
    for (let i = 0; i < parts.length; i += 2) {
      const expr = parts[i].trim();
      const negated = expr.startsWith('!');
      const key = negated ? expr.slice(1) : expr;
      
      let value = false;
      if (key === 'editorFocus') value = context.editorFocus ?? false;
      else if (key === 'sidebarFocus') value = context.sidebarFocus ?? false;
      else if (key === 'inputFocus') value = context.inputFocus ?? false;
      else if (key === 'modalOpen') value = context.modalOpen ?? false;
      else if (key === 'always') value = true;

      const result = negated ? !value : value;

      if (i === 0) {
        // First expression
        if (!result && parts.length > 1 && parts[1] === '&&') return false;
        if (result && parts.length > 1 && parts[1] === '||') return true;
      } else {
        // Subsequent expressions
        const op = parts[i - 1];
        if (op === '&&' && !result) return false;
        if (op === '||' && result) return true;
      }
    }

    return true;
  }

  findMatchingKeybinding(event: KeyboardEvent): KeybindingItem | null {
    // Get platform-specific key (renderer-safe: no `process` access)
    const ua = typeof navigator !== 'undefined' ? navigator.userAgent.toLowerCase() : '';
    const plat = typeof navigator !== 'undefined' ? (navigator.platform || '').toLowerCase() : '';
    const isMac = plat.includes('mac') || ua.includes('mac os');
    const isWin = plat.includes('win') || ua.includes('windows');
    const platform: 'mac' | 'win' | 'linux' = isMac ? 'mac' : isWin ? 'win' : 'linux';

    for (const binding of this.keybindings) {
      // Check platform-specific override
      const key = (platform === 'mac' && binding.mac) ? binding.mac :
                  (platform === 'win' && binding.win) ? binding.win :
                  (platform === 'linux' && binding.linux) ? binding.linux :
                  binding.key;

      if (!key) continue;

      const parts = this.parseKeybinding(key);
      if (this.matchesKeybinding(event, parts) && this.evaluateWhen(binding.when)) {
        return binding;
      }
    }

    return null;
  }
}

// Main KeybindingManager class
export class KeybindingManager {
  private commandRegistry = new CommandRegistry();
  private matcher = new KeybindingMatcher();
  private keydownHandler: ((e: KeyboardEvent) => void) | null = null;
  private isEnabled = true;

  constructor() {
    this.setupGlobalKeyListener();
  }

  private setupGlobalKeyListener() {
    this.keydownHandler = (event: KeyboardEvent) => {
      if (!this.isEnabled) return;

      // Don't handle if user is typing in an input/textarea
      const target = event.target as HTMLElement;
      if (
        target.tagName === 'INPUT' ||
        target.tagName === 'TEXTAREA' ||
        target.isContentEditable
      ) {
        // Only handle if it's a modifier-only key combo
        if (!event.metaKey && !event.ctrlKey && !event.altKey) {
          return;
        }
      }

      const binding = this.matcher.findMatchingKeybinding(event);
      if (binding) {
        event.preventDefault();
        event.stopPropagation();
        this.commandRegistry.execute(binding.command, binding.args);
      }
    };

    document.addEventListener('keydown', this.keydownHandler, true);
  }

  loadKeybindings(keybindings: KeybindingItem[]) {
    this.matcher.updateKeybindings(keybindings);
  }

  updateContext(context: Partial<ContextState>) {
    this.matcher.updateContext(context);
  }

  registerCommand(command: string, handler: CommandHandler) {
    this.commandRegistry.register(command, handler);
  }

  unregisterCommand(command: string) {
    this.commandRegistry.unregister(command);
  }

  executeCommand(command: string, args?: unknown) {
    return this.commandRegistry.execute(command, args);
  }

  setEnabled(enabled: boolean) {
    this.isEnabled = enabled;
  }

  dispose() {
    if (this.keydownHandler) {
      document.removeEventListener('keydown', this.keydownHandler, true);
      this.keydownHandler = null;
    }
  }
}

// Singleton instance
let keybindingManagerInstance: KeybindingManager | null = null;

export function getKeybindingManager(): KeybindingManager {
  if (!keybindingManagerInstance) {
    keybindingManagerInstance = new KeybindingManager();
  }
  return keybindingManagerInstance;
}
