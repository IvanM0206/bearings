import { render } from '@testing-library/react';
import { describe, expect, it } from 'vitest';
import { Markdown } from './Markdown';
import { safeHref } from './safeUrl';

const labels = { image: 'Image' };

function html(source: string): HTMLElement {
  const { container } = render(<Markdown source={source} labels={labels} />);
  return container.firstElementChild as HTMLElement;
}

describe('safeHref', () => {
  it.each([
    ['https://example.com/a?b=1', 'https://example.com/a?b=1'],
    ['http://example.com', 'http://example.com/'],
    ['mailto:a@example.com', 'mailto:a@example.com'],
  ])('allows %s', (input, out) => {
    expect(safeHref(input)).toBe(out);
  });

  it.each([
    'javascript:alert(1)',
    'JaVaScRiPt:alert(1)',
    'java\tscript:alert(1)',
    'java\nscript:alert(1)',
    ' javascript:alert(1)',
    'data:text/html,<script>alert(1)</script>',
    'vbscript:msgbox(1)',
    'file:///etc/passwd',
    'ftp://example.com',
    'blob:https://example.com/x',
    '//example.com/x',
    '/relative/path',
    '#fragment',
    'example.com',
    '',
    'https://user:pass@example.com/',
    'https://',
  ])('rejects %j', (input) => {
    expect(safeHref(input)).toBeNull();
  });

  it('rejects absurdly long URLs', () => {
    expect(safeHref('https://example.com/' + 'a'.repeat(5000))).toBeNull();
  });
});

describe('Markdown rendering', () => {
  it('renders the common syntax', () => {
    const el = html('# Title\n\nSome **bold** and *italic* and `code`.\n\n- one\n- two\n\n1. a\n2. b\n\n> quote\n\n---\n\n```\nlet x = 1;\n```');
    expect(el.querySelector('h3')?.textContent).toBe('Title'); // the page title is the h1
    expect(el.querySelector('strong')?.textContent).toBe('bold');
    expect(el.querySelector('em')?.textContent).toBe('italic');
    expect(el.querySelectorAll('ul li')).toHaveLength(2);
    expect(el.querySelectorAll('ol li')).toHaveLength(2);
    expect(el.querySelector('blockquote')).not.toBeNull();
    expect(el.querySelector('pre code')?.textContent).toContain('let x = 1;');
    expect(el.querySelector('hr')).not.toBeNull();
  });

  it('renders tables', () => {
    const el = html('| a | b |\n|---|:-:|\n| 1 | 2 |');
    expect(el.querySelectorAll('th')).toHaveLength(2);
    expect(el.querySelector('td.md-align-center')?.textContent).toBe('2');
  });

  it('keeps tight list items free of paragraphs', () => {
    const el = html('- one\n- two');
    expect(el.querySelector('li p')).toBeNull();
  });

  it('shows raw HTML as text, never as elements', () => {
    const el = html('<script>window.hacked = 1</script>\n\n<img src=x onerror="window.hacked = 2">\n\n<b onclick="x()">bold</b> <iframe src="https://evil.example"></iframe>');
    expect(el.querySelector('script, img, iframe, b')).toBeNull();
    expect(el.textContent).toContain('<script>window.hacked = 1</script>');
    expect(el.textContent).toContain('onerror=');
    expect((window as unknown as Record<string, unknown>)['hacked']).toBeUndefined();
    expect(el.innerHTML).not.toMatch(/<(script|img|iframe)/i);
  });

  it('escapes HTML inside inline code and fences', () => {
    const el = html('`<img src=x onerror=alert(1)>`\n\n```html\n<script>alert(1)</script>\n```');
    expect(el.querySelector('img, script')).toBeNull();
    expect(el.textContent).toContain('<script>alert(1)</script>');
  });

  it('does not make javascript: and data: links clickable', () => {
    const el = html('[a](javascript:alert(1)) [b](JAVASCRIPT:alert(1)) [c](data:text/html;base64,PHNjcmlwdD4=) [d](vbscript:x) [e](java&#x09;script:alert(1)) [f](/relative) [g](#frag)');
    expect(el.querySelectorAll('a')).toHaveLength(0);
    expect(el.innerHTML).not.toMatch(/href/i);
    for (const word of ['a', 'b', 'c', 'd', 'e', 'f', 'g']) expect(el.textContent).toContain(word);
  });

  it('does not make reference-style or autolinked javascript: URLs clickable', () => {
    const el = html('[x][1] <javascript:alert(1)>\n\n[1]: javascript:alert(1)');
    expect(el.querySelectorAll('a')).toHaveLength(0);
  });

  it('opens http, https and mailto links safely', () => {
    const el = html('[web](https://example.com/x) [plain](http://example.com) [mail](mailto:a@example.com)');
    const anchors = [...el.querySelectorAll('a')];
    expect(anchors).toHaveLength(3);
    for (const a of anchors) {
      expect(a.getAttribute('target')).toBe('_blank');
      expect(a.getAttribute('rel')).toBe('noopener noreferrer nofollow');
    }
    expect(anchors[0]?.getAttribute('href')).toBe('https://example.com/x');
  });

  it('does not autolink bare URLs', () => {
    expect(html('see https://example.com').querySelector('a')).toBeNull();
  });

  it('never loads images: alt text becomes a link or plain text', () => {
    const el = html('![logo](https://tracker.example/pixel.png) ![](https://example.com/a.png) ![x](javascript:alert(1)) ![local](data:image/png;base64,AAAA)');
    expect(el.querySelector('img')).toBeNull();
    const links = [...el.querySelectorAll('a.md-image-link')];
    expect(links.map((a) => a.textContent)).toEqual(['logo', 'Image']);
    expect(links[0]?.getAttribute('href')).toBe('https://tracker.example/pixel.png');
    expect(el.innerHTML).not.toMatch(/src=/i);
    expect(el.textContent).toContain('x');
  });

  it('does not let attributes injected through link text or titles through', () => {
    const el = html('[x" onmouseover="alert(1)](https://example.com "t\\" onclick=\\"alert(1)")');
    const a = el.querySelector('a');
    expect(a?.getAttributeNames().sort()).toEqual(['href', 'rel', 'target']);
  });

  it('survives hostile nesting and huge input', () => {
    const el = html('> '.repeat(500) + 'deep\n\n' + '['.repeat(2000) + 'x' + ']'.repeat(2000) + '\n\n' + 'a'.repeat(200000));
    expect(el.textContent?.length ?? 0).toBeGreaterThan(0);
  });

  it('renders nothing risky for empty input', () => {
    expect(html('').innerHTML).toBe('');
  });
});
