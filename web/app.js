(() => {
  'use strict';

  const STATES       = ['needed', 'check', 'not_needed'];
  const STATE_LABELS = { needed: 'Needed', check: 'Check', not_needed: 'Not Needed' };
  const NO_GROUP     = 'No Group';

  let items           = [];
  let groups          = [];   // real groups only; NO_GROUP is virtual
  let syncEnabled     = true;
  let collapsedGroups = {};
  let hideNotNeeded   = false;

  const drag = { active: false, id: null, srcGroup: null };

  const gc           = document.getElementById('groups-container');
  const emptyEl      = document.getElementById('empty-state');
  const addForm      = document.getElementById('add-form');
  const newInput     = document.getElementById('new-item-input');
  const groupSel     = document.getElementById('group-select');
  const syncTog      = document.getElementById('sync-toggle');
  const banner       = document.getElementById('offline-banner');
  const resetBtn     = document.getElementById('reset-btn');
  const groupsBtn    = document.getElementById('groups-btn');
  const collapseAll      = document.getElementById('collapse-all-btn');
  const expandAll        = document.getElementById('expand-all-btn');
  const hideNotNeededBtn = document.getElementById('hide-not-needed-btn');

  // ── Reset modal
  const resetModal   = document.getElementById('reset-modal');
  const resetCancel  = document.getElementById('reset-cancel');
  const resetConfirm = document.getElementById('reset-confirm');

  // ── Groups modal
  const groupsModal  = document.getElementById('groups-modal');
  const groupsList   = document.getElementById('groups-modal-list');
  const groupsForm   = document.getElementById('groups-modal-form');
  const groupsInput  = document.getElementById('groups-modal-input');
  const groupsClose  = document.getElementById('groups-modal-close');

  // ────────────────────────────────────────────────────────────────
  // Pure helpers  (mirrored in app.test.js — keep in sync)
  // ────────────────────────────────────────────────────────────────
  function nextState(s) {
    return STATES[(STATES.indexOf(s) + 1) % STATES.length];
  }

  function itemsForGroup(group) {
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
   * groupsForRender returns the ordered render list:
   * real groups first, then the virtual NO_GROUP appended only if
   * at least one item carries that group.
   */
  function groupsForRender() {
    const hasOrphans = items.some(i => i.group === NO_GROUP);
    return hasOrphans ? [...groups, NO_GROUP] : [...groups];
  }

  // ────────────────────────────────────────────────────────────────
  // API
  // ────────────────────────────────────────────────────────────────
  async function api(method, path, body) {
    const opts = { method, headers: { 'Content-Type': 'application/json' } };
    if (body !== undefined) opts.body = JSON.stringify(body);
    const res = await fetch(path, opts);
    if (res.status === 204) return null;
    return res.json();
  }

  async function loadConfig() {
    const cfg = await api('GET', '/api/config').catch(() => null);
    groups = cfg?.groups || [];
    rebuildGroupSelect();
  }

  async function fetchItems() {
    const data = await api('GET', '/api/items').catch(() => []);
    items = data || [];
    render();
  }

  async function syncToServer() {
    const data = await api('POST', '/api/sync', items).catch(() => null);
    if (data) { items = data; render(); }
  }

  // ────────────────────────────────────────────────────────────────
  // Group select (footer)
  // ────────────────────────────────────────────────────────────────
  function rebuildGroupSelect() {
    const prev = groupSel.value;
    groupSel.innerHTML = '';
    // Never offer NO_GROUP as an add target
    groups.forEach(g => {
      const opt = document.createElement('option');
      opt.value = g;
      opt.textContent = g.length > 14 ? g.slice(0, 13) + '\u2026' : g;
      opt.title = g;
      groupSel.appendChild(opt);
    });
    if (prev && groups.includes(prev)) groupSel.value = prev;
  }

  // ────────────────────────────────────────────────────────────────
  // Mutations
  // ────────────────────────────────────────────────────────────────
  async function addItem(name, group) {
    const tempId = 'local-' + Date.now();
    items.push({
      id: tempId, name, group,
      state: 'needed', completed: false,
      order: items.filter(i => i.group === group).length,
      created_at: new Date().toISOString()
    });
    render();
    if (syncEnabled) {
      const saved = await api('POST', '/api/items', { name, group }).catch(() => null);
      if (saved) {
        const idx = items.findIndex(i => i.id === tempId);
        if (idx !== -1) items[idx] = saved;
        render();
      }
    }
  }

  async function toggleComplete(id) {
    const item = items.find(i => i.id === id);
    if (!item) return;
    item.completed = !item.completed;
    render();
    if (syncEnabled) api('PATCH', `/api/items/${id}`, { completed: item.completed });
  }

  async function cycleState(id) {
    const item = items.find(i => i.id === id);
    if (!item) return;
    item.state = nextState(item.state);
    render();
    if (syncEnabled) api('PATCH', `/api/items/${id}`, { state: item.state });
  }

  async function deleteItem(id) {
    items = items.filter(i => i.id !== id);
    render();
    if (syncEnabled) api('DELETE', `/api/items/${id}`);
  }

  async function moveItem(id, toGroup, orderIds) {
    const item = items.find(i => i.id === id);
    if (!item) return;
    item.group = toGroup;
    orderIds.forEach((oid, idx) => {
      const it = items.find(i => i.id === oid);
      if (it) it.order = idx;
    });
    render();
    if (syncEnabled) api('POST', '/api/move', { id, group: toGroup, order_ids: orderIds });
  }

  async function reorderWithinGroup(group, orderIds) {
    orderIds.forEach((oid, idx) => {
      const it = items.find(i => i.id === oid);
      if (it) it.order = idx;
    });
    render();
    if (syncEnabled) api('POST', '/api/reorder', { group, ids: orderIds });
  }

  // ────────────────────────────────────────────────────────────────
  // Collapse / Expand all
  // ────────────────────────────────────────────────────────────────
  function setAllCollapsed(collapsed) {
    groupsForRender().forEach(g => { collapsedGroups[g] = collapsed; });
    render();
  }

  collapseAll.addEventListener('click', () => setAllCollapsed(true));
  expandAll.addEventListener('click',   () => setAllCollapsed(false));

  // ────────────────────────────────────────────────────────────────
  // Hide "Not Needed" toggle
  // ────────────────────────────────────────────────────────────────
  hideNotNeededBtn.addEventListener('click', () => {
    hideNotNeeded = !hideNotNeeded;
    hideNotNeededBtn.classList.toggle('active', hideNotNeeded);
    hideNotNeededBtn.setAttribute('aria-pressed', String(hideNotNeeded));
    hideNotNeededBtn.title = hideNotNeeded
      ? "Show \u2018Not Needed\u2019 items"
      : "Hide \u2018Not Needed\u2019 items and empty groups";
    render();
  });

  // ────────────────────────────────────────────────────────────────
  // Reset modal
  // ────────────────────────────────────────────────────────────────
  function openResetModal()  { resetModal.classList.remove('hidden'); resetConfirm.focus(); }
  function closeResetModal() { resetModal.classList.add('hidden'); }

  async function doReset() {
    closeResetModal();
    items.forEach(item => { item.completed = false; item.state = 'check'; });
    render();
    if (syncEnabled) {
      const data = await api('POST', '/api/reset').catch(() => null);
      if (data) { items = data; render(); }
    }
  }

  resetBtn.addEventListener('click',     openResetModal);
  resetCancel.addEventListener('click',  closeResetModal);
  resetConfirm.addEventListener('click', doReset);
  resetModal.addEventListener('click', e => { if (e.target === resetModal) closeResetModal(); });

  document.addEventListener('keydown', e => {
    if (e.key === 'Escape') { closeResetModal(); closeGroupsModal(); }
  });

  // ────────────────────────────────────────────────────────────────
  // Groups modal
  // ────────────────────────────────────────────────────────────────
  function renderGroupsList() {
    groupsList.innerHTML = '';
    if (groups.length === 0) {
      const li = document.createElement('li');
      li.className   = 'groups-modal-empty';
      li.textContent = 'No groups yet';
      groupsList.appendChild(li);
      return;
    }
    groups.forEach(g => {
      const li  = document.createElement('li');
      li.className = 'groups-modal-item';

      const nameEl = document.createElement('span');
      nameEl.className   = 'groups-modal-name';
      nameEl.textContent = g;

      const del = document.createElement('button');
      del.className = 'groups-modal-delete';
      del.title     = `Remove group \u201c${g}\u201d`;
      del.setAttribute('aria-label', `Remove group ${g}`);
      del.innerHTML = `<svg viewBox="0 0 16 16" fill="none" stroke="currentColor"
        stroke-width="2" stroke-linecap="round">
        <path d="M3 3l10 10M13 3L3 13"/></svg>`;
      del.addEventListener('click', () => removeGroup(g));

      li.appendChild(nameEl);
      li.appendChild(del);
      groupsList.appendChild(li);
    });
  }

  function openGroupsModal() {
    renderGroupsList();
    groupsModal.classList.remove('hidden');
    groupsInput.value = '';
    groupsInput.focus();
  }

  function closeGroupsModal() { groupsModal.classList.add('hidden'); }

  async function addGroup(name) {
    if (!name || name === NO_GROUP || groups.includes(name)) return;
    // Optimistic
    groups.push(name);
    rebuildGroupSelect();
    renderGroupsList();
    render();
    if (syncEnabled) {
      const data = await api('POST', '/api/config/groups', { name }).catch(() => null);
      if (data?.groups) {
        groups = data.groups;
        rebuildGroupSelect();
        renderGroupsList();
        render();
      }
    }
  }

  async function removeGroup(name) {
    // Optimistic local: move items in deleted group to NO_GROUP
    items.forEach(item => { if (item.group === name) item.group = NO_GROUP; });
    groups = groups.filter(g => g !== name);
    rebuildGroupSelect();
    renderGroupsList();
    render();
    if (syncEnabled) {
      // POST /api/config/groups/remove returns { groups, items }
      const data = await api('POST', '/api/config/groups/remove', { name }).catch(() => null);
      if (data) {
        if (data.groups) groups = data.groups;
        if (data.items)  items  = data.items;
        rebuildGroupSelect();
        renderGroupsList();
        render();
      }
    }
  }

  groupsBtn.addEventListener('click', openGroupsModal);
  groupsClose.addEventListener('click', closeGroupsModal);
  groupsModal.addEventListener('click', e => { if (e.target === groupsModal) closeGroupsModal(); });

  groupsForm.addEventListener('submit', e => {
    e.preventDefault();
    const name = groupsInput.value.trim();
    if (!name) return;
    groupsInput.value = '';
    addGroup(name);
    groupsInput.focus();
  });

  // ────────────────────────────────────────────────────────────────
  // Render
  // ────────────────────────────────────────────────────────────────
  function render() {
    gc.innerHTML = '';
    emptyEl.classList.toggle('hidden', items.length > 0);

    const renderList = groupsForRender();

    renderList.forEach(group => {
      const allGroupItems = itemsForGroup(group);
      const groupItems    = hideNotNeeded
        ? allGroupItems.filter(i => i.state !== 'not_needed')
        : allGroupItems;

      // When hiding not-needed, skip groups that are now empty.
      if (hideNotNeeded && groupItems.length === 0) return;
      const isVirtual  = group === NO_GROUP;
      const isOpen     = !collapsedGroups[group];

      // Skip empty real groups during drag so they can still act as drop targets.
      // Always show NO_GROUP section (it only appears when it has items).
      const section = document.createElement('div');
      section.className     = 'group-section' + (isVirtual ? ' group-section--nogroup' : '');
      section.dataset.group = group;

      const header = document.createElement('div');
      header.className = 'group-header' + (isOpen ? ' open' : '') + (isVirtual ? ' group-header--nogroup' : '');
      header.innerHTML = `
        <span class="group-title">${esc(group)}</span>
        <span class="group-meta">
          <span class="group-count">${hideNotNeeded ? groupItems.length + '/' + allGroupItems.length : groupItems.length}</span>
          <svg class="group-chevron" viewBox="0 0 16 16" fill="none"
               stroke="currentColor" stroke-width="2"
               stroke-linecap="round" stroke-linejoin="round">
            <path d="M4 6l4 4 4-4"/>
          </svg>
        </span>`;
      header.addEventListener('click', () => {
        collapsedGroups[group] = !collapsedGroups[group];
        render();
      });

      const body = document.createElement('div');
      body.className     = 'group-body' + (isOpen ? '' : ' collapsed');
      body.dataset.group = group;

      if (groupItems.length === 0) {
        const hint = document.createElement('div');
        hint.className   = 'group-empty';
        hint.textContent = isVirtual ? 'No orphaned items' : 'No items';
        body.appendChild(hint);
      } else {
        const ul = document.createElement('ul');
        ul.className     = 'item-list';
        ul.dataset.group = group;
        groupItems.forEach(item => ul.appendChild(buildRow(item)));
        body.appendChild(ul);
      }

      section.appendChild(header);
      section.appendChild(body);
      gc.appendChild(section);
    });
  }

  // ────────────────────────────────────────────────────────────────
  // Build item row
  // ────────────────────────────────────────────────────────────────
  function buildRow(item) {
    const li = document.createElement('li');
    li.className     = 'item-row' + (item.completed ? ' completed' : '');
    li.dataset.id    = item.id;
    li.dataset.group = item.group;

    li.innerHTML = `
      <span class="drag-handle" title="Drag to reorder or move group">
        <svg viewBox="0 0 14 14" fill="currentColor">
          <circle cx="4" cy="3"  r="1.2"/><circle cx="10" cy="3"  r="1.2"/>
          <circle cx="4" cy="7"  r="1.2"/><circle cx="10" cy="7"  r="1.2"/>
          <circle cx="4" cy="11" r="1.2"/><circle cx="10" cy="11" r="1.2"/>
        </svg>
      </span>
      <input type="checkbox" class="item-checkbox" data-id="${item.id}"
             ${item.completed ? 'checked' : ''}
             aria-label="Mark ${esc(item.name)} complete">
      <span class="item-content">
        <span class="item-name" data-id="${item.id}">${esc(item.name)}</span>
        <span class="state-badge" data-state="${item.state}" data-id="${item.id}">
          ${STATE_LABELS[item.state]}
        </span>
      </span>
      <button class="delete-btn" data-id="${item.id}" aria-label="Delete ${esc(item.name)}">
        <svg viewBox="0 0 20 20" fill="none" stroke="currentColor"
             stroke-width="1.8" stroke-linecap="round" stroke-linejoin="round">
          <path d="M3 6h14M8 6V4h4v2M5 6l1 11h8l1-11"/>
        </svg>
      </button>`;

    attachDragToHandle(li, li.querySelector('.drag-handle'));
    return li;
  }

  // ────────────────────────────────────────────────────────────────
  // Drag  (handle-only, mouse + touch)
  // ────────────────────────────────────────────────────────────────
  function attachDragToHandle(row, handle) {
    function clearIndicators() {
      gc.querySelectorAll('.drag-over-above,.drag-over-below')
        .forEach(el => el.classList.remove('drag-over-above', 'drag-over-below'));
      gc.querySelectorAll('.drag-target')
        .forEach(el => el.classList.remove('drag-target'));
    }

    function pointerStart() {
      drag.active   = true;
      drag.id       = row.dataset.id;
      drag.srcGroup = row.dataset.group;
      row.classList.add('dragging');
    }

    function pointerMove(clientX, clientY) {
      if (!drag.active) return;
      clearIndicators();
      const el = document.elementFromPoint(clientX, clientY);
      if (!el) return;

      const targetSection = el.closest('.group-section');
      const targetGroup   = targetSection?.dataset?.group;

      if (targetGroup && targetGroup !== drag.srcGroup) {
        targetSection.classList.add('drag-target');
        if (collapsedGroups[targetGroup]) {
          collapsedGroups[targetGroup] = false;
          render();
          const reRendered = gc.querySelector(`[data-id="${drag.id}"]`);
          if (reRendered) reRendered.classList.add('dragging');
          const sec = gc.querySelector(
            `.group-section[data-group="${CSS.escape(targetGroup)}"]`
          );
          if (sec) sec.classList.add('drag-target');
        }
        return;
      }

      const targetRow = el.closest('.item-row');
      if (targetRow && targetRow.dataset.id !== drag.id) {
        const rect = targetRow.getBoundingClientRect();
        targetRow.classList.add(
          clientY < rect.top + rect.height / 2 ? 'drag-over-above' : 'drag-over-below'
        );
      }
    }

    function pointerEnd(clientX, clientY) {
      if (!drag.active) return;
      drag.active = false;
      const el = document.elementFromPoint(clientX, clientY);
      clearIndicators();
      gc.querySelectorAll('.item-row.dragging').forEach(r => r.classList.remove('dragging'));

      if (!el) { drag.id = null; return; }

      const targetSection = el.closest('.group-section');
      const targetGroup   = targetSection?.dataset?.group;
      const targetRow     = el.closest('.item-row');

      if (targetGroup && targetGroup !== drag.srcGroup) {
        const destIds = itemsForGroup(targetGroup).map(i => i.id);
        destIds.push(drag.id);
        moveItem(drag.id, targetGroup, destIds);
      } else if (
        targetRow &&
        targetRow.dataset.id !== drag.id &&
        targetRow.dataset.group === drag.srcGroup
      ) {
        const ids     = itemsForGroup(drag.srcGroup).map(i => i.id);
        const fromIdx = ids.indexOf(drag.id);
        const rect    = targetRow.getBoundingClientRect();
        let   toIdx   = ids.indexOf(targetRow.dataset.id);
        if (clientY >= rect.top + rect.height / 2) toIdx++;
        ids.splice(fromIdx, 1);
        ids.splice(Math.max(0, fromIdx < toIdx ? toIdx - 1 : toIdx), 0, drag.id);
        reorderWithinGroup(drag.srcGroup, ids);
      } else {
        render();
      }
      drag.id = null; drag.srcGroup = null;
    }

    // Mouse
    handle.addEventListener('mousedown', e => {
      e.preventDefault();
      pointerStart();
      const onMove = e => pointerMove(e.clientX, e.clientY);
      const onUp   = e => {
        pointerEnd(e.clientX, e.clientY);
        document.removeEventListener('mousemove', onMove);
        document.removeEventListener('mouseup',   onUp);
      };
      document.addEventListener('mousemove', onMove);
      document.addEventListener('mouseup',   onUp);
    });

    // Touch
    handle.addEventListener('touchstart', e => {
      pointerStart();
      const onMove = e => {
        const t = e.touches[0];
        pointerMove(t.clientX, t.clientY);
        e.preventDefault();
      };
      const onEnd = e => {
        const t = e.changedTouches[0];
        pointerEnd(t.clientX, t.clientY);
        handle.removeEventListener('touchmove', onMove);
        handle.removeEventListener('touchend',  onEnd);
      };
      handle.addEventListener('touchmove', onMove, { passive: false });
      handle.addEventListener('touchend',  onEnd);
    }, { passive: true });
  }

  // ────────────────────────────────────────────────────────────────
  // Event delegation  (list)
  // ────────────────────────────────────────────────────────────────
  gc.addEventListener('click', e => {
    if (drag.id) return;
    const cb    = e.target.closest('.item-checkbox');
    const name  = e.target.closest('.item-name');
    const badge = e.target.closest('.state-badge');
    const del   = e.target.closest('.delete-btn');
    if (cb)    { toggleComplete(cb.dataset.id);  return; }
    if (name)  { cycleState(name.dataset.id);    return; }
    if (badge) { cycleState(badge.dataset.id);   return; }
    if (del)   { deleteItem(del.dataset.id);     return; }
  });

  addForm.addEventListener('submit', e => {
    e.preventDefault();
    const name = newInput.value.trim();
    if (!name) return;
    const group = groupSel.value || groups[0] || NO_GROUP;
    newInput.value = '';
    addItem(name, group);
    newInput.focus();
  });

  syncTog.addEventListener('change', async () => {
    syncEnabled = syncTog.checked;
    banner.classList.toggle('hidden', syncEnabled);
    if (syncEnabled) await syncToServer();
  });

  // ────────────────────────────────────────────────────────────────
  // Init
  // ────────────────────────────────────────────────────────────────
  (async () => {
    await loadConfig();
    await fetchItems();
  })();

})();
