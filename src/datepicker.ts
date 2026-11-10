import { icon } from './ui';

const triggerFormat = new Intl.DateTimeFormat('es-PE', { day: '2-digit', month: 'short', year: 'numeric' });
const monthFormat = new Intl.DateTimeFormat('es-PE', { month: 'long', year: 'numeric' });
const weekdays = ['lu', 'ma', 'mi', 'ju', 'vi', 'sá', 'do'];
const dayKeys: Record<string, number> = { ArrowLeft: -1, ArrowRight: 1, ArrowUp: -7, ArrowDown: 7 };

const syncers = new WeakMap<HTMLInputElement, () => void>();
let popover: HTMLElement | undefined;
let activeInput: HTMLInputElement | undefined;
let viewYear = 0;
let viewMonth = 0;

function iso(date: Date): string {
  return [date.getFullYear(), String(date.getMonth() + 1).padStart(2, '0'), String(date.getDate()).padStart(2, '0')].join('-');
}

function parseISO(value: string): Date | undefined {
  const match = /^(\d{4})-(\d{2})-(\d{2})$/.exec(value);
  if (!match) return undefined;
  const date = new Date(Number(match[1]), Number(match[2]) - 1, Number(match[3]));
  return Number.isNaN(date.getTime()) ? undefined : date;
}

function shiftDate(date: Date, days: number): Date {
  return new Date(date.getFullYear(), date.getMonth(), date.getDate() + days);
}

function formatValue(value: string): string {
  const date = parseISO(value);
  return date ? triggerFormat.format(date).replace(/\./g, '') : 'Seleccionar fecha';
}

function capitalize(value: string): string {
  return value.charAt(0).toUpperCase() + value.slice(1);
}

function decorate(input: HTMLInputElement): void {
  if (input.dataset.datepicker) return;
  input.dataset.datepicker = 'ready';

  const wrapper = document.createElement('span');
  wrapper.className = 'datepicker';
  input.replaceWith(wrapper);
  wrapper.append(input);

  const display = document.createElement('span');
  display.className = 'datepicker-display';
  display.setAttribute('aria-hidden', 'true');
  const value = document.createElement('span');
  value.className = 'datepicker-value';
  const glyph = document.createElement('span');
  glyph.className = 'datepicker-glyph';
  glyph.innerHTML = icon('calendar');
  display.append(value, glyph);
  wrapper.prepend(display);

  input.type = 'text';
  input.readOnly = true;
  input.autocomplete = 'off';
  input.inputMode = 'none';
  input.classList.add('datepicker-input');
  input.setAttribute('aria-haspopup', 'dialog');
  input.setAttribute('aria-expanded', 'false');

  const sync = (): void => { value.textContent = formatValue(input.value); };
  syncers.set(input, sync);
  sync();

  input.addEventListener('click', () => {
    if (popover && !popover.hidden && activeInput === input) close(true);
    else openFor(input);
  });
  input.addEventListener('keydown', event => {
    if (event.key === 'Enter' || event.key === ' ' || event.key === 'ArrowDown') {
      event.preventDefault();
      openFor(input);
    }
  });
  input.form?.addEventListener('reset', () => requestAnimationFrame(sync));
}

function ensurePopover(): HTMLElement {
  if (popover) return popover;
  popover = document.createElement('div');
  popover.className = 'datepicker-popover';
  popover.setAttribute('role', 'dialog');
  popover.setAttribute('aria-label', 'Seleccionar fecha');
  popover.hidden = true;
  popover.addEventListener('click', event => {
    const target = event.target as HTMLElement;
    const day = target.closest<HTMLElement>('[data-day]');
    if (day) {
      choose(day.dataset.day || '');
      return;
    }
    if (target.closest('[data-previous]')) shiftMonth(-1);
    else if (target.closest('[data-next]')) shiftMonth(1);
    else if (target.closest('[data-today]')) {
      const today = new Date();
      viewYear = today.getFullYear();
      viewMonth = today.getMonth();
      choose(iso(today));
    }
  });
  popover.addEventListener('keydown', event => {
    if (event.key === 'Escape') {
      event.preventDefault();
      close(true);
      return;
    }
    const day = (event.target as HTMLElement).closest<HTMLElement>('[data-day]');
    const delta = dayKeys[event.key];
    if (!day || delta === undefined) return;
    event.preventDefault();
    navigate(day.dataset.day || iso(new Date()), delta);
  });
  return popover;
}

function openFor(input: HTMLInputElement): void {
  const dialog = input.closest('dialog[open]');
  const host = dialog instanceof HTMLElement ? dialog : document.body;
  const panel = ensurePopover();
  if (panel.parentElement !== host) host.append(panel);

  activeInput = input;
  input.setAttribute('aria-expanded', 'true');
  const current = parseISO(input.value) ?? new Date();
  viewYear = current.getFullYear();
  viewMonth = current.getMonth();
  render(input.value ? input.value : iso(current));
  panel.hidden = false;
  position();

  document.addEventListener('pointerdown', onOutside, true);
  window.addEventListener('resize', position);
  window.addEventListener('scroll', position, true);
}

function close(returnFocus: boolean): void {
  if (!popover || popover.hidden) return;
  popover.hidden = true;
  document.removeEventListener('pointerdown', onOutside, true);
  window.removeEventListener('resize', position);
  window.removeEventListener('scroll', position, true);
  activeInput?.setAttribute('aria-expanded', 'false');
  const input = activeInput;
  activeInput = undefined;
  if (returnFocus) input?.focus();
}

function onOutside(event: PointerEvent): void {
  const target = event.target;
  if (!(target instanceof Node)) return;
  if (popover?.contains(target)) return;
  if (activeInput?.closest('.datepicker')?.contains(target)) return;
  close(false);
}

function position(): void {
  if (!popover || !activeInput || popover.hidden) return;
  const anchor = activeInput.closest<HTMLElement>('.datepicker') ?? activeInput;
  const rect = anchor.getBoundingClientRect();
  const panel = popover.getBoundingClientRect();
  const gap = 8;
  let top = rect.bottom + gap;
  if (top + panel.height > window.innerHeight - 8) top = rect.top - panel.height - gap;
  top = Math.max(8, Math.min(top, window.innerHeight - panel.height - 8));
  let left = rect.left;
  if (left + panel.width > window.innerWidth - 8) left = window.innerWidth - panel.width - 8;
  left = Math.max(8, left);
  popover.style.top = `${Math.round(top)}px`;
  popover.style.left = `${Math.round(left)}px`;
}

function shiftMonth(delta: number): void {
  const date = new Date(viewYear, viewMonth + delta, 1);
  viewYear = date.getFullYear();
  viewMonth = date.getMonth();
  render(iso(date));
  position();
}

function navigate(from: string, delta: number): void {
  const base = parseISO(from) ?? new Date();
  const target = shiftDate(base, delta);
  viewYear = target.getFullYear();
  viewMonth = target.getMonth();
  render(iso(target));
  position();
}

function choose(value: string): void {
  const input = activeInput;
  if (!input || !value) return;
  input.value = value;
  input.dispatchEvent(new Event('input', { bubbles: true }));
  input.dispatchEvent(new Event('change', { bubbles: true }));
  syncers.get(input)?.();
  close(true);
}

function render(focusValue?: string): void {
  if (!popover) return;
  const first = new Date(viewYear, viewMonth, 1);
  const offset = (first.getDay() + 6) % 7;
  const start = shiftDate(first, -offset);
  const selected = activeInput?.value ?? '';
  const today = iso(new Date());
  let cells = '';
  for (let index = 0; index < 42; index += 1) {
    const date = shiftDate(start, index);
    const value = iso(date);
    const classes = ['datepicker-day'];
    if (date.getMonth() !== viewMonth) classes.push('is-outside');
    if (value === today) classes.push('is-today');
    if (value === selected) classes.push('is-selected');
    cells += `<button type="button" class="${classes.join(' ')}" data-day="${value}" tabindex="-1">${date.getDate()}</button>`;
  }
  popover.innerHTML = `<div class="datepicker-head">
      <button type="button" data-previous aria-label="Mes anterior">${icon('previous')}</button>
      <strong>${capitalize(monthFormat.format(first))}</strong>
      <button type="button" data-next aria-label="Mes siguiente">${icon('next')}</button>
    </div>
    <div class="datepicker-weekdays">${weekdays.map(day => `<span>${day}</span>`).join('')}</div>
    <div class="datepicker-grid">${cells}</div>
    <div class="datepicker-foot"><button type="button" data-today>Hoy</button></div>`;

  const buttons = popover.querySelectorAll<HTMLButtonElement>('[data-day]');
  buttons.forEach(button => { button.tabIndex = -1; });
  const ready = (focusValue && popover.querySelector<HTMLButtonElement>(`[data-day="${focusValue}"]`))
    || popover.querySelector<HTMLButtonElement>('[data-day].is-selected')
    || popover.querySelector<HTMLButtonElement>('[data-day].is-today')
    || buttons[0];
  if (ready) ready.tabIndex = 0;
  if (focusValue && ready) ready.focus();
}

export function installDatePickers(): void {
  const scan = (root: ParentNode): void => {
    root.querySelectorAll<HTMLInputElement>('input[type="date"]:not([data-datepicker])').forEach(decorate);
  };
  scan(document);
  new MutationObserver(records => {
    records.forEach(record => {
      record.addedNodes.forEach(node => {
        if (!(node instanceof HTMLElement)) return;
        if (node instanceof HTMLInputElement && node.matches('input[type="date"]:not([data-datepicker])')) decorate(node);
        scan(node);
      });
    });
  }).observe(document.body, { childList: true, subtree: true });
}
