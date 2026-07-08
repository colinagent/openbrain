import { EditorState, StateField, type Extension } from '@codemirror/state';
import { Decoration, DecorationSet, EditorView, WidgetType } from '@codemirror/view';
import {
  areComposerPlanStatesEqual,
  mapComposerPlanState,
  type ComposerPlanState,
} from '../../utils/chatPlanBlock';

type ComposerPlanLineFieldValue = {
  planState: ComposerPlanState;
  decorations: DecorationSet;
};

type ComposerPlanLineConfig = {
  planState: ComposerPlanState;
  onRemove: () => void;
  onStateChange?: (planState: ComposerPlanState) => void;
};

class ComposerPlanLineWidget extends WidgetType {
  private readonly onRemove: () => void;

  constructor(onRemove: () => void) {
    super();
    this.onRemove = onRemove;
  }

  eq(other: ComposerPlanLineWidget): boolean {
    return other.onRemove === this.onRemove;
  }

  ignoreEvent(): boolean {
    return true;
  }

  toDOM(): HTMLElement {
    const root = document.createElement('div');
    root.className = 'cm-md-composer-plan-line';

    const pill = document.createElement('span');
    pill.className = 'ui-capsule-pill cm-md-composer-plan-chip';

    const label = document.createElement('span');
    label.className = 'truncate text-sm';
    label.textContent = 'Plan';
    pill.appendChild(label);

    const remove = document.createElement('button');
    remove.type = 'button';
    remove.className = 'cm-md-composer-plan-chip-remove icon-gutter-btn-sm icon-button-inline';
    remove.setAttribute('aria-label', 'Remove selected skill');
    remove.title = 'Remove selected skill';
    remove.textContent = '×';
    remove.addEventListener('mousedown', (event) => {
      event.preventDefault();
      event.stopPropagation();
    });
    remove.addEventListener('click', (event) => {
      event.preventDefault();
      event.stopPropagation();
      this.onRemove();
      const view = EditorView.findFromDOM(root);
      view?.focus();
    });
    pill.appendChild(remove);

    root.addEventListener('mousedown', (event) => {
      event.preventDefault();
      event.stopPropagation();
      EditorView.findFromDOM(root)?.focus();
    });

    root.appendChild(pill);
    return root;
  }
}

function clampPlanState(planState: ComposerPlanState, docLength: number): ComposerPlanState {
  const clamp = (pos: number) => Math.max(0, Math.min(pos, docLength));
  const clampSpacer = (spacer: ComposerPlanState['beforeSpacer']) => {
    if (!spacer) {
      return null;
    }
    const from = clamp(spacer.from);
    const to = clamp(spacer.to);
    return {
      ...spacer,
      from: Math.min(from, to),
      to: Math.max(from, to),
    };
  };
  return {
    anchor: clamp(planState.anchor),
    beforeSpacer: clampSpacer(planState.beforeSpacer),
    afterSpacer: clampSpacer(planState.afterSpacer),
  };
}

function buildPlanLineDecorations(
  state: EditorState,
  planState: ComposerPlanState,
  onRemove: () => void,
): DecorationSet {
  return Decoration.set([
    Decoration.widget({
      widget: new ComposerPlanLineWidget(onRemove),
      block: true,
      side: 1,
    }).range(Math.max(0, Math.min(planState.anchor, state.doc.length))),
  ], true);
}

export function createComposerPlanLineExtension(config: ComposerPlanLineConfig): Extension {
  const field = StateField.define<ComposerPlanLineFieldValue>({
    create(state) {
      const planState = clampPlanState(config.planState, state.doc.length);
      return {
        planState,
        decorations: buildPlanLineDecorations(state, planState, config.onRemove),
      };
    },
    update(value, tr) {
      const planState = tr.docChanged
        ? clampPlanState(mapComposerPlanState(value.planState, tr.changes), tr.state.doc.length)
        : clampPlanState(value.planState, tr.state.doc.length);
      return {
        planState,
        decorations: buildPlanLineDecorations(tr.state, planState, config.onRemove),
      };
    },
    provide: (stateField) => EditorView.decorations.from(stateField, (value) => value.decorations),
  });

  let lastReportedPlanState = config.planState;
  return [
    field,
    EditorView.updateListener.of((update) => {
      if (!config.onStateChange || !update.docChanged) {
        return;
      }
      const nextPlanState = update.state.field(field).planState;
      if (areComposerPlanStatesEqual(lastReportedPlanState, nextPlanState)) {
        return;
      }
      lastReportedPlanState = nextPlanState;
      config.onStateChange(nextPlanState);
    }),
  ];
}
