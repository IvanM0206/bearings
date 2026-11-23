import MarkdownIt, { type Token } from 'markdown-it';
import { Fragment, useMemo, type ReactNode } from 'react';
import { safeHref } from './safeUrl';

// Markdown is notebook content, so it is untrusted. It is never turned into
// an HTML string: markdown-it parses it (raw HTML disabled) and this file
// builds React elements for a fixed set of tags. Text is escaped by React,
// every URL goes through safeHref, and nothing is injected as markup.
const md = new MarkdownIt({ html: false, linkify: false, typographer: false, breaks: true });
md.validateLink = (url) => safeHref(url) !== null;

interface Node {
  token: Token;
  children: Node[];
}

function buildTree(tokens: Token[]): Node[] {
  const root: Node = { token: tokens[0] as Token, children: [] };
  const stack: Node[] = [root];
  for (const token of tokens) {
    const top = stack[stack.length - 1] as Node;
    if (token.nesting === 1) {
      const node: Node = { token, children: [] };
      top.children.push(node);
      stack.push(node);
    } else if (token.nesting === -1) {
      if (stack.length > 1) stack.pop();
    } else {
      top.children.push({ token, children: [] });
    }
  }
  return root.children;
}

export interface MarkdownLabels {
  /** Text for an image without alt text. */
  image: string;
}

const LINK_REL = 'noopener noreferrer nofollow';

function attr(t: Token, name: string): string {
  const v = t.attrGet(name);
  return v === null ? '' : String(v);
}

function alignClass(token: Token): string | undefined {
  const style = attr(token, 'style');
  const m = /text-align:\s*(left|right|center)/.exec(style);
  return m ? `md-align-${m[1] ?? ''}` : undefined;
}

function renderInline(tokens: Token[], labels: MarkdownLabels): ReactNode[] {
  return renderNodes(buildTree(tokens), labels);
}

function link(href: string | null, content: ReactNode, className?: string): ReactNode {
  if (href === null) return <span>{content}</span>;
  return (
    <a href={href} target="_blank" rel={LINK_REL} className={className}>
      {content}
    </a>
  );
}

function textOf(tokens: Token[]): string {
  return tokens.map((t) => (t.type === 'text' || t.type === 'code_inline' ? t.content : textOf(t.children ?? []))).join('');
}

function renderNode(n: Node, key: number, labels: MarkdownLabels): ReactNode {
  const t = n.token;
  const kids = () => renderNodes(n.children, labels);
  switch (t.type) {
    case 'paragraph_open':
      return t.hidden ? <Fragment key={key}>{kids()}</Fragment> : <p key={key}>{kids()}</p>;
    case 'inline':
      return <Fragment key={key}>{renderInline(t.children ?? [], labels)}</Fragment>;
    case 'heading_open': {
      // The page title is the h1: note headings start at h3.
      const level = Math.min(Number(t.tag.slice(1)) + 2, 6);
      const Tag = `h${level}` as 'h3' | 'h4' | 'h5' | 'h6';
      return <Tag key={key}>{kids()}</Tag>;
    }
    case 'bullet_list_open':
      return <ul key={key}>{kids()}</ul>;
    case 'ordered_list_open': {
      const start = Number(attr(t, 'start') || 1);
      return (
        <ol key={key} {...(Number.isInteger(start) && start !== 1 ? { start } : {})}>
          {kids()}
        </ol>
      );
    }
    case 'list_item_open':
      return <li key={key}>{kids()}</li>;
    case 'blockquote_open':
      return <blockquote key={key}>{kids()}</blockquote>;
    case 'hr':
      return <hr key={key} />;
    case 'fence':
    case 'code_block':
      return (
        // A scrollable region must be reachable with the keyboard.
        // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
        <pre key={key} tabIndex={0}>
          <code>{t.content}</code>
        </pre>
      );
    case 'table_open':
      return (
        // eslint-disable-next-line jsx-a11y/no-noninteractive-tabindex
        <div key={key} className="md-table" tabIndex={0}>
          <table>{kids()}</table>
        </div>
      );
    case 'thead_open':
      return <thead key={key}>{kids()}</thead>;
    case 'tbody_open':
      return <tbody key={key}>{kids()}</tbody>;
    case 'tr_open':
      return <tr key={key}>{kids()}</tr>;
    case 'th_open':
      return (
        <th key={key} scope="col" className={alignClass(t)}>
          {kids()}
        </th>
      );
    case 'td_open':
      return (
        <td key={key} className={alignClass(t)}>
          {kids()}
        </td>
      );
    case 'text':
      return <Fragment key={key}>{t.content}</Fragment>;
    case 'code_inline':
      return <code key={key}>{t.content}</code>;
    case 'softbreak':
    case 'hardbreak':
      return <br key={key} />;
    case 'strong_open':
      return <strong key={key}>{kids()}</strong>;
    case 'em_open':
      return <em key={key}>{kids()}</em>;
    case 's_open':
      return <s key={key}>{kids()}</s>;
    case 'link_open':
      return <Fragment key={key}>{link(safeHref(attr(t, 'href')), kids())}</Fragment>;
    case 'image': {
      // Remote images are never loaded (privacy, and the CSP forbids them):
      // show the alt text as a link instead.
      const alt = textOf(t.children ?? []).trim() || labels.image;
      return (
        <Fragment key={key}>{link(safeHref(attr(t, 'src')), alt, 'md-image-link')}</Fragment>
      );
    }
    default:
      // Unknown or disabled token (html_block, html_inline never occur with
      // html:false). Render nothing for leaf tokens, children for containers.
      return n.children.length > 0 ? <Fragment key={key}>{kids()}</Fragment> : null;
  }
}

function renderNodes(nodes: Node[], labels: MarkdownLabels): ReactNode[] {
  return nodes.map((n, i) => renderNode(n, i, labels));
}

/** Renders Markdown source as safe React elements. */
export function Markdown({ source, labels, className }: { source: string; labels: MarkdownLabels; className?: string }) {
  const nodes = useMemo(() => {
    const tokens = md.parse(source, {});
    return tokens.length === 0 ? [] : renderNodes(buildTree(tokens), labels);
  }, [source, labels]);
  return <div className={className ? `md ${className}` : 'md'}>{nodes}</div>;
}
