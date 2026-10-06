import {createApp, h} from 'vue';
import BubbleNode from './BubbleNode.vue';

function mount(props: Record<string, any>) {
  const root = document.createElementNS('http://www.w3.org/2000/svg', 'svg');
  document.body.append(root);
  const app = createApp({
    render: () => h(BubbleNode, {id: 'n1', x: 0, y: 0, r: 40, contributors: 3, countText: '3', countFontSize: 14, ...props}),
  });
  app.mount(root);
  onTestFinished(() => {
    app.unmount();
    root.remove();
  });
  return root.querySelector<SVGGElement>('g.node');
}

test('a bubble is a control that names its article', () => {
  const node = mount({});
  expect(node.getAttribute('role')).toBe('button');
  expect(node.getAttribute('tabindex')).toBe('0');
  expect(node.getAttribute('aria-label')).toContain('3 contributors');
});

test('the hidden root is no tab stop and says nothing', () => {
  const node = mount({isHidden: true, isTombstoned: true});
  expect(node.getAttribute('role')).toBeNull();
  expect(node.getAttribute('tabindex')).toBe('-1');
  expect(node.getAttribute('aria-hidden')).toBe('true');
  expect(node.getAttribute('aria-label')).toBeNull();
  expect(node.getAttribute('aria-pressed')).toBeNull();
});

test('in compare mode a pick is spoken with its place in the comparison', () => {
  const node = mount({isCompareMode: true, compareState: 'second'});
  expect(node.getAttribute('aria-label')).toContain('selected for comparison (2 of 2)');
  expect(node.getAttribute('aria-pressed')).toBe('true');
});
