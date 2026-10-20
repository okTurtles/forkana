import {currentGraphState, graphStateOf, pushGraphState, replaceGraphState} from './graph-history.ts';

describe('graph history state', () => {
  beforeEach(() => {
    window.history.replaceState({view: 'bubble', owner: 'bob', repo: 'moon-landing'}, '', '/subject/Moon?selected=bob%2Fmoon-landing');
  });

  test('graphStateOf reads only the graph part', () => {
    expect(graphStateOf(null)).toEqual({});
    expect(graphStateOf({view: 'bubble'})).toEqual({});
    expect(graphStateOf({graph: {poc: '1-2'}})).toEqual({poc: '1-2'});
  });

  test('replaceGraphState merges into the entry and keeps the page state and url', () => {
    replaceGraphState({poc: '1-2', scrollY: 300});
    replaceGraphState({scrollY: null});
    expect(window.history.state).toEqual({view: 'bubble', owner: 'bob', repo: 'moon-landing', graph: {poc: '1-2', scrollY: null}});
    expect(window.location.pathname + window.location.search).toBe('/subject/Moon?selected=bob%2Fmoon-landing');
  });

  test('pushGraphState starts a new entry with only the given graph state', () => {
    replaceGraphState({poc: '1-2'});
    const before = window.history.length;
    pushGraphState({detail: true});
    expect(window.history.length).toBe(before + 1);
    expect(currentGraphState()).toEqual({detail: true});
    expect(window.history.state.owner).toBe('bob');
  });
});
