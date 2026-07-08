import { useRef, useEffect, type ReactNode } from 'react';
import ReactMarkdown, { type Components } from 'react-markdown';
import remarkGfm from 'remark-gfm';
import { renderMermaidSVG } from 'beautiful-mermaid';
import { findMarkdownHighlightRanges } from '../Editor/codemirror/utils/markdownHighlight';

function isExternalMarkdownHref(href: string): boolean {
  return /^(https?:\/\/|mailto:)/i.test(href);
}

function buildClassName(...values: Array<string | undefined | null | false>): string {
  return values.filter(Boolean).join(' ');
}

const MARKDOWN_COMPONENTS: Components = {
  mark: ({ children, className, ...props }) => {
    return (
      <mark {...props} className={buildClassName('op-md-highlight', className)}>
        {children}
      </mark>
    );
  },
  a: ({ href, children, className, onClick, ...props }) => {
    const normalizedHref = typeof href === 'string' ? href.trim() : '';
    if (!isExternalMarkdownHref(normalizedHref)) {
      return (
        <span
          className={buildClassName('op-activity-panel-md-link-disabled', className)}
          title={normalizedHref || undefined}
        >
          {children}
        </span>
      );
    }

    return (
      <a
        {...props}
        href={normalizedHref}
        className={buildClassName('op-activity-panel-md-link', className)}
        target="_blank"
        rel="noopener noreferrer"
        onClick={(event) => {
          onClick?.(event);
          if (event.defaultPrevented) {
            return;
          }
          event.preventDefault();
          event.stopPropagation();
          window.open(normalizedHref, '_blank', 'noopener,noreferrer');
        }}
      >
        {children}
      </a>
    );
  },
  img: ({ alt, src }) => {
    const label = (alt || '').trim() || 'Image';
    const description = (typeof src === 'string' ? src.trim() : '') || '';
    return (
      <span
        className="op-activity-panel-md-image-placeholder"
        title={description || undefined}
      >
        {description ? `🖼 ${label} (${description})` : `🖼 ${label}`}
      </span>
    );
  },
  code: ({ className, children }) => {
    const lang = (className || '').replace(/^language-/, '');
    const code = extractText(children).replace(/\n$/, '');
    if (lang === 'mermaid' && code) {
      return <MermaidBlock code={code} />;
    }
    return (
      <code className={className}>
        {children}
      </code>
    );
  },
  pre: ({ children }) => {
    return <pre className="op-activity-panel-md-pre">{children}</pre>;
  },
};

type MarkdownAstNode = {
  type?: string;
  value?: string;
  children?: MarkdownAstNode[];
  data?: {
    hName?: string;
    hProperties?: Record<string, unknown>;
  };
};

function createHighlightNode(children: MarkdownAstNode[]): MarkdownAstNode {
  return {
    type: 'highlight',
    children,
    data: {
      hName: 'mark',
      hProperties: {
        className: 'op-md-highlight',
      },
    },
  };
}

function splitHighlightTextNode(value: string): MarkdownAstNode[] {
  const ranges = findMarkdownHighlightRanges(value);
  if (ranges.length === 0) {
    return [{ type: 'text', value }];
  }

  const nodes: MarkdownAstNode[] = [];
  let cursor = 0;
  for (const range of ranges) {
    if (cursor < range.from) {
      nodes.push({ type: 'text', value: value.slice(cursor, range.from) });
    }
    nodes.push(createHighlightNode([
      { type: 'text', value: value.slice(range.from + 2, range.to - 2) },
    ]));
    cursor = range.to;
  }
  if (cursor < value.length) {
    nodes.push({ type: 'text', value: value.slice(cursor) });
  }
  return nodes;
}

function transformHighlightNodes(node: MarkdownAstNode): void {
  if (!node.children || node.type === 'code' || node.type === 'inlineCode') {
    return;
  }

  const nextChildren: MarkdownAstNode[] = [];
  for (const child of node.children) {
    if (child.type === 'text' && typeof child.value === 'string') {
      nextChildren.push(...splitHighlightTextNode(child.value));
      continue;
    }
    transformHighlightNodes(child);
    nextChildren.push(child);
  }
  node.children = nextChildren;
}

function remarkHighlight() {
  return (tree: MarkdownAstNode) => {
    transformHighlightNodes(tree);
  };
}

const REMARK_PLUGINS = [remarkGfm, remarkHighlight];

function extractText(node: ReactNode): string {
  if (typeof node === 'string') return node;
  if (typeof node === 'number') return String(node);
  if (Array.isArray(node)) return node.map(extractText).join('');
  if (node && typeof node === 'object' && 'props' in node) {
    return extractText((node as any).props.children);
  }
  return '';
}

function MermaidBlock({ code }: { code: string }) {
  const containerRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    const el = containerRef.current;
    if (!el) return;
    try {
      const svg = renderMermaidSVG(code, {
        bg: 'var(--color-editor-bg)',
        fg: 'var(--color-editor-fg)',
        accent: 'var(--color-accent)',
        border: 'var(--color-border)',
        transparent: true,
      });
      el.innerHTML = svg;
      el.classList.remove('op-activity-panel-md-mermaid-error');
    } catch (error) {
      el.classList.add('op-activity-panel-md-mermaid-error');
      const message = error instanceof Error ? error.message : String(error);
      el.textContent = `Invalid mermaid diagram: ${message}`;
    }
  }, [code]);

  return <div ref={containerRef} className="op-activity-panel-md-mermaid" />;
}

export function ActivityMarkdownView({
  text,
  className,
}: {
  text: string;
  className?: string;
}) {
  if (!text.trim()) {
    return null;
  }

  return (
    <div className={buildClassName('op-activity-panel-text', 'op-activity-panel-markdown', className)}>
      <ReactMarkdown remarkPlugins={REMARK_PLUGINS} components={MARKDOWN_COMPONENTS}>
        {text}
      </ReactMarkdown>
    </div>
  );
}
