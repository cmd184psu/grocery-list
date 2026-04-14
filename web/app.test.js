/**
 * app.test.js — pure-logic unit tests (no DOM required)
 *
 * Run with:  node --test app.test.js
 * Requires Node.js >= 18 (built-in test runner + assert).
 */
import { describe, it } from 'node:test';
import assert           from 'node:assert/strict';

// ────────────────────────────────────────────────────────────────
// Inline copies of pure helpers from app.js (no DOM dependency).
// Keep these in sync with app.js.
// ────────────────────────────────────────────────────────────────

const STATES       = ['needed', 'check', 'not_needed'];
const NO_GROUP     = 'No Group';

function nextState(s) {
  return STATES[(STATES.indexOf(s) + 1) % STATES.length];
}

function itemsForGroup(items, group) {
  return [...items]
    .filter(i => i.group === group)
    .sort((a, b) =>
      a.order !== b.order
        ? a.order - b.order
        : new Date(a.created_at) - new Date(b.created_at)
    );
}

function esc(str) {
  return String(str)
    .replace(/&/g, '&amp;').replace(/</g, '&lt;')
    .replace(/>/g, '&gt;').replace(/"/g, '&quot;');
}

/**
 * groupsForRender returns the ordered list of groups to render, appending the
 * virtual "No Group" section if any item carries that group and it has items.
 */
function groupsForRender(groups, items) {
  const has = items.some(i => i.group === NO_GROUP);
  return has ? [...groups, NO_GROUP] : [...groups];
}

/**
 * Apply a reset: set all items to completed=false, state='check'.
 */
function applyReset(items) {
  return items.map(i => ({ ...i, completed: false, state: 'check' }));
}

/**
 * removeGroup moves items from the deleted group to NO_GROUP and
 * returns updated { groups, items }.
 */
function removeGroup(groups, items, name) {
  const newGroups = groups.filter(g => g !== name);
  const newItems  = items.map(i =>
    i.group === name ? { ...i, group: NO_GROUP } : i
  );
  return { groups: newGroups, items: newItems };
}

// ────────────────────────────────────────────────────────────────
// Tests
// ────────────────────────────────────────────────────────────────

describe('nextState', () => {
  it('cycles needed → check', () => assert.equal(nextState('needed'), 'check'));
  it('cycles check → not_needed', () => assert.equal(nextState('check'), 'not_needed'));
  it('cycles not_needed → needed', () => assert.equal(nextState('not_needed'), 'needed'));
  it('handles unknown state gracefully (returns needed)', () => {
    // indexOf returns -1, (-1+1)%3 = 0 → 'needed'
    assert.equal(nextState('bogus'), 'needed');
  });
});

describe('itemsForGroup', () => {
  const items = [
    { id: '1', group: 'A', order: 2, created_at: '2024-01-01T00:00:00Z' },
    { id: '2', group: 'A', order: 0, created_at: '2024-01-01T00:00:00Z' },
    { id: '3', group: 'B', order: 0, created_at: '2024-01-01T00:00:00Z' },
    { id: '4', group: 'A', order: 1, created_at: '2024-01-01T00:00:00Z' },
  ];

  it('filters to the correct group', () => {
    const result = itemsForGroup(items, 'A');
    assert.equal(result.length, 3);
    result.forEach(i => assert.equal(i.group, 'A'));
  });

  it('sorts by order ascending', () => {
    const result = itemsForGroup(items, 'A');
    assert.deepEqual(result.map(i => i.id), ['2', '4', '1']);
  });

  it('returns empty for unknown group', () => {
    assert.deepEqual(itemsForGroup(items, 'Z'), []);
  });

  it('does not mutate source array', () => {
    const copy = [...items];
    itemsForGroup(items, 'A');
    assert.deepEqual(items, copy);
  });
});

describe('esc', () => {
  it('escapes &', () => assert.equal(esc('a&b'), 'a&amp;b'));
  it('escapes <', () => assert.equal(esc('<tag>'), '&lt;tag&gt;'));
  it('escapes "', () => assert.equal(esc('say "hi"'), 'say &quot;hi&quot;'));
  it('passes plain strings unchanged', () => assert.equal(esc('hello'), 'hello'));
  it('coerces numbers', () => assert.equal(esc(42), '42'));
});

describe('groupsForRender', () => {
  const baseGroups = ['Produce', 'Dairy'];

  it('returns groups unchanged when no NoGroup items', () => {
    const items = [{ group: 'Produce' }, { group: 'Dairy' }];
    assert.deepEqual(groupsForRender(baseGroups, items), baseGroups);
  });

  it('appends No Group when orphaned items exist', () => {
    const items = [{ group: 'Produce' }, { group: NO_GROUP }];
    const result = groupsForRender(baseGroups, items);
    assert.equal(result[result.length - 1], NO_GROUP);
    assert.equal(result.length, baseGroups.length + 1);
  });

  it('does not append No Group if already empty', () => {
    assert.deepEqual(groupsForRender(baseGroups, []), baseGroups);
  });

  it('does not mutate the source groups array', () => {
    const src = ['A', 'B'];
    groupsForRender(src, [{ group: NO_GROUP }]);
    assert.equal(src.length, 2);
  });
});

describe('applyReset', () => {
  it('sets all items to completed=false and state=check', () => {
    const items = [
      { id: '1', completed: true,  state: 'not_needed' },
      { id: '2', completed: false, state: 'needed' },
    ];
    const result = applyReset(items);
    result.forEach(i => {
      assert.equal(i.completed, false);
      assert.equal(i.state, 'check');
    });
  });

  it('does not mutate the original array', () => {
    const items = [{ id: '1', completed: true, state: 'needed' }];
    applyReset(items);
    assert.equal(items[0].completed, true);
  });

  it('returns an array of the same length', () => {
    const items = Array.from({ length: 5 }, (_, i) => ({
      id: String(i), completed: true, state: 'needed'
    }));
    assert.equal(applyReset(items).length, 5);
  });
});

describe('removeGroup', () => {
  const groups = ['Produce', 'Dairy', 'Frozen'];
  const items  = [
    { id: '1', group: 'Produce' },
    { id: '2', group: 'Dairy' },
    { id: '3', group: 'Produce' },
  ];

  it('removes the named group from the list', () => {
    const { groups: g } = removeGroup(groups, items, 'Produce');
    assert.ok(!g.includes('Produce'));
  });

  it('moves items from deleted group to No Group', () => {
    const { items: i } = removeGroup(groups, items, 'Produce');
    const orphans = i.filter(x => x.id === '1' || x.id === '3');
    orphans.forEach(o => assert.equal(o.group, NO_GROUP));
  });

  it('leaves items in other groups untouched', () => {
    const { items: i } = removeGroup(groups, items, 'Produce');
    const dairy = i.find(x => x.id === '2');
    assert.equal(dairy.group, 'Dairy');
  });

  it('does not mutate the source arrays', () => {
    const origGroups = [...groups];
    const origItems  = items.map(i => ({ ...i }));
    removeGroup(groups, items, 'Dairy');
    assert.deepEqual(groups, origGroups);
    assert.deepEqual(items.map(i => i.group), origItems.map(i => i.group));
  });

  it('handles removing a non-existent group gracefully', () => {
    const { groups: g, items: i } = removeGroup(groups, items, 'Bakery');
    assert.deepEqual(g, groups);
    assert.deepEqual(i.map(x => x.group), items.map(x => x.group));
  });
});
