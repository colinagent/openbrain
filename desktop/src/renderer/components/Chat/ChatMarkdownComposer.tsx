import React, {
  forwardRef,
  useEffect,
  useImperativeHandle,
  useRef,
} from 'react';
import type { EditorView } from '@codemirror/view';
import { createMarkdownEditor, type MarkdownEditorInstance } from '../Editor/codemirror/setup';
import type { ImageActivation } from '../Editor/codemirror/livePreviewPlugin';
import type { ComposerPlanState } from '../../utils/chatPlanBlock';
import { createComposerPlanLineExtension } from './composerPlanLineExtension';

export type ChatMarkdownComposerHandle = {
  focus: () => void;
  blur: () => void;
  insertText: (text: string) => void;
  getView: () => EditorView | null;
};

type ChatMarkdownComposerProps = {
  value: string;
  readOnly: boolean;
  planBlock: ComposerPlanState | null;
  placeholder: string;
  documentPath: string | null;
  dragOver: boolean;
  onChange: (value: string) => void;
  onSelectionChange: (anchor: number) => void;
  onFocus: () => void;
  onImageActivate: (image: ImageActivation) => void;
  onImageDelete: (image: ImageActivation) => void;
  onRemovePlanLine: () => void;
  onPlanBlockStateChange: (planState: ComposerPlanState) => void;
  onKeyDownCapture: (event: React.KeyboardEvent<HTMLDivElement>) => void;
  onPasteCapture: (event: React.ClipboardEvent<HTMLDivElement>) => void;
  onDragOver: (event: React.DragEvent<HTMLDivElement>) => void;
  onDragLeave: (event: React.DragEvent<HTMLDivElement>) => void;
  onDrop: (event: React.DragEvent<HTMLDivElement>) => void;
};

export const ChatMarkdownComposer = forwardRef<ChatMarkdownComposerHandle, ChatMarkdownComposerProps>(
  function ChatMarkdownComposer({
    value,
    readOnly,
    planBlock,
    placeholder,
    documentPath,
    dragOver,
    onChange,
    onSelectionChange,
    onFocus,
    onImageActivate,
    onImageDelete,
    onRemovePlanLine,
    onPlanBlockStateChange,
    onKeyDownCapture,
    onPasteCapture,
    onDragOver,
    onDragLeave,
    onDrop,
  }, ref) {
    const containerRef = useRef<HTMLDivElement | null>(null);
    const editorRef = useRef<MarkdownEditorInstance | null>(null);
    const onChangeRef = useRef(onChange);
    const onSelectionChangeRef = useRef(onSelectionChange);
    const onFocusRef = useRef(onFocus);
    const onImageActivateRef = useRef(onImageActivate);
    const onImageDeleteRef = useRef(onImageDelete);
    const onRemovePlanLineRef = useRef(onRemovePlanLine);
    const onPlanBlockStateChangeRef = useRef(onPlanBlockStateChange);

    useEffect(() => {
      onChangeRef.current = onChange;
    }, [onChange]);

    useEffect(() => {
      onSelectionChangeRef.current = onSelectionChange;
    }, [onSelectionChange]);

    useEffect(() => {
      onFocusRef.current = onFocus;
    }, [onFocus]);

    useEffect(() => {
      onImageActivateRef.current = onImageActivate;
    }, [onImageActivate]);

    useEffect(() => {
      onImageDeleteRef.current = onImageDelete;
    }, [onImageDelete]);

    useEffect(() => {
      onRemovePlanLineRef.current = onRemovePlanLine;
    }, [onRemovePlanLine]);

    useEffect(() => {
      onPlanBlockStateChangeRef.current = onPlanBlockStateChange;
    }, [onPlanBlockStateChange]);

    useImperativeHandle(ref, () => ({
      focus: () => {
        editorRef.current?.focus();
      },
      blur: () => {
        editorRef.current?.blur();
      },
      insertText: (text: string) => {
        editorRef.current?.insertAtSelection(text);
      },
      getView: () => editorRef.current?.getView() || null,
    }), []);

    useEffect(() => {
      if (!containerRef.current) {
        return;
      }

      editorRef.current?.destroy();
      containerRef.current.textContent = '';
      editorRef.current = createMarkdownEditor(containerRef.current, {
        initialContent: value,
        documentPath,
        onContentChange: (nextValue) => {
          onChangeRef.current(nextValue);
        },
        onSelectionChange: (selection) => {
          onSelectionChangeRef.current(selection.head);
        },
        onFocusChange: (focused) => {
          if (focused) {
            onFocusRef.current();
          }
        },
        onImageActivate: (image) => {
          onImageActivateRef.current(image);
        },
        onImageDelete: (image) => {
          onImageDeleteRef.current(image);
        },
        showImageDeleteButton: false,
        footerWidgetExtension: planBlock
          ? createComposerPlanLineExtension({
            planState: planBlock,
            onRemove: () => onRemovePlanLineRef.current(),
            onStateChange: (nextPlanState) => onPlanBlockStateChangeRef.current(nextPlanState),
          })
          : null,
        livePreview: true,
        readOnly,
      });

      return () => {
        editorRef.current?.destroy();
        editorRef.current = null;
      };
    }, [documentPath, readOnly]);

    useEffect(() => {
      const editor = editorRef.current;
      if (!editor) {
        return;
      }
      editor.setFooterWidgetExtension(
        planBlock
          ? createComposerPlanLineExtension({
            planState: planBlock,
            onRemove: () => onRemovePlanLineRef.current(),
            onStateChange: (nextPlanState) => onPlanBlockStateChangeRef.current(nextPlanState),
          })
          : null
      );
    }, [planBlock]);

    useEffect(() => {
      const editor = editorRef.current;
      if (!editor) {
        return;
      }
      if (editor.getContent() !== value) {
        editor.setContent(value);
      }
    }, [value]);

    return (
      <div
        className={`chat-markdown-composer relative flex-1 min-w-0 overflow-visible ${dragOver ? 'conversation-textarea-drag-over' : ''}`}
        onKeyDownCapture={onKeyDownCapture}
        onPasteCapture={onPasteCapture}
        onDragOverCapture={onDragOver}
        onDragLeaveCapture={onDragLeave}
        onDropCapture={onDrop}
      >
        {value.length === 0 && !planBlock && (
          <div className="chat-markdown-composer-placeholder pointer-events-none absolute text-base text-tertiary-text">
            {placeholder}
          </div>
        )}
        <div
          ref={containerRef}
          className="chat-markdown-composer-editor min-h-[60px] rounded-lg"
        />
      </div>
    );
  },
);
