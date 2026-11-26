function attachClientTableScroll(tableWrapper, top) {
  const body = tableWrapper.querySelector('.ant-table-body');
  if (!body) return () => {};

  const events = new AbortController();
  const {signal} = events;
  const mirroredPositions = new WeakMap();
  let drag = null;
  let suppressClick = false;

  function mirror(source, target) {
    if (!top.hidden && Math.abs(source.scrollLeft - target.scrollLeft) >= 1) {
      target.scrollLeft = source.scrollLeft;
      mirroredPositions.set(target, target.scrollLeft);
    }
  }

  // Scroll events are queued. An event from mirroring must not undo newer input.
  function syncScroll(source, target) {
    const mirrored = mirroredPositions.get(source);
    mirroredPositions.delete(source);
    if (source.scrollLeft !== mirrored) mirror(source, target);
  }

  function moveBody(left) {
    body.scrollLeft = left;
    mirror(body, top);
  }

  function resizeTrack() {
    const overflow = body.scrollWidth - body.clientWidth;
    top.hidden = overflow <= 1;
    body.classList.toggle('bo-clients-scrollable', !top.hidden);
    if (top.hidden) return;
    top.dir = getComputedStyle(body).direction;
    top.firstElementChild.style.width = (top.clientWidth + overflow) + 'px';
    mirror(body, top);
  }

  const resize = new ResizeObserver(resizeTrack);
  resize.observe(body);
  const table = body.querySelector('table');
  if (table) resize.observe(table);
  resizeTrack();

  top.addEventListener('scroll', () => syncScroll(top, body), {signal, passive: true});
  body.addEventListener('scroll', () => syncScroll(body, top), {signal, passive: true});

  function wheelSideways(event) {
    if (top.hidden || event.ctrlKey || event.deltaX || !event.deltaY ||
        (!event.shiftKey && event.currentTarget !== top)) return;
    const scale = event.deltaMode === WheelEvent.DOM_DELTA_LINE ? 16
      : event.deltaMode === WheelEvent.DOM_DELTA_PAGE ? body.clientWidth : 1;
    const direction = top.dir === 'rtl' ? -1 : 1;
    const previous = body.scrollLeft;
    moveBody(previous + event.deltaY * scale * direction);
    if (event.shiftKey || body.scrollLeft !== previous) event.preventDefault();
  }

  top.addEventListener('wheel', wheelSideways, {signal, passive: false});
  body.addEventListener('wheel', wheelSideways, {signal, passive: false});

  body.addEventListener('pointerdown', event => {
    suppressClick = false;
    // Leave controls, modified clicks/text selection and native touch scrolling alone.
    if (top.hidden || event.pointerType !== 'mouse' || event.button !== 0 ||
        event.shiftKey || event.ctrlKey || event.altKey || event.metaKey) {
      return;
    }
    const control = event.target.closest('a, button, input, select, textarea, label, summary, [tabindex], [role="button"], [contenteditable], .ant-table-row-expand-icon');
    if (control && control !== body && body.contains(control)) return;
    drag = {
      id: event.pointerId,
      x: event.clientX,
      y: event.clientY,
      left: body.scrollLeft,
      scale: body.getBoundingClientRect().width / body.offsetWidth || 1,
      active: false,
    };
  }, {signal});

  window.addEventListener('pointermove', event => {
    if (!drag || drag.id !== event.pointerId) return;
    const delta = event.clientX - drag.x;
    if (!drag.active) {
      if (Math.abs(event.clientY - drag.y) > Math.max(5, Math.abs(delta))) {
        drag = null;
        return;
      }
      if (Math.abs(delta) < 5) return;
      drag.active = true;
      suppressClick = true;
      body.setPointerCapture(event.pointerId);
      body.classList.add('bo-clients-dragging');
      window.getSelection()?.removeAllRanges();
    }
    event.preventDefault();
    moveBody(drag.left - delta / drag.scale);
  }, {signal, passive: false});

  function endDrag(event) {
    if (!drag || (event && event.pointerId !== drag.id)) return;
    drag = null;
    body.classList.remove('bo-clients-dragging');
  }

  window.addEventListener('pointerup', endDrag, {signal});
  window.addEventListener('pointercancel', endDrag, {signal});
  window.addEventListener('blur', () => endDrag(), {signal});
  body.addEventListener('lostpointercapture', endDrag, {signal});
  body.addEventListener('click', event => {
    if (!suppressClick || !event.detail) return;
    suppressClick = false;
    event.preventDefault();
    event.stopImmediatePropagation();
  }, {signal, capture: true});

  return () => {
    endDrag();
    resize.disconnect();
    events.abort();
  };
}
