import { errorMessage } from './shared';

const paths = {
  home: '<path d="m3 10 9-7 9 7v10a1 1 0 0 1-1 1h-5v-7H9v7H4a1 1 0 0 1-1-1Z"/>',
  calendar: '<rect x="3" y="5" width="18" height="16" rx="3"/><path d="M3 10h18M8 3v4m8-4v4m-7 8 2 2 4-4"/>',
  people: '<circle cx="9" cy="8" r="3"/><path d="M3 20v-2a5 5 0 0 1 5-5h2a5 5 0 0 1 5 5v2Zm13-15a3 3 0 0 1 0 6m2 3a4 4 0 0 1 3 4v2"/>',
  wallet: '<path d="M3 7h15a3 3 0 0 1 3 3v8a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2V7Zm0 0a3 3 0 0 1 3-3h11m4 8h-5a2 2 0 0 0 0 4h5"/>',
  plus: '<path d="M12 5v14M5 12h14"/>',
  arrow: '<path d="M5 12h14m-6-6 6 6-6 6"/>',
  next: '<path d="m9 18 6-6-6-6"/>',
  previous: '<path d="m15 18-6-6 6-6"/>',
  search: '<circle cx="10.5" cy="10.5" r="5.5"/><path d="m15 15 4 4"/>',
  check: '<path d="m5 12 4 4L19 6"/>',
  lock: '<rect x="5" y="10" width="14" height="11" rx="2"/><path d="M8 10V7a4 4 0 0 1 8 0v3m-4 4v3"/>',
  logout: '<path d="M12 4H5a2 2 0 0 0-2 2v12a2 2 0 0 0 2 2h7m3-4 4-4-4-4m4 4H9"/>',
  person: '<circle cx="12" cy="8" r="3.5"/><path d="M5 21v-2a5 5 0 0 1 5-5h4a5 5 0 0 1 5 5v2"/>',
  tooth: '<path d="M12 5C8 2 4 4 4 8c0 3 2 4 3 8s2 5 3 5c2 0 0-6 2-6s0 6 2 6c1 0 2-1 3-5s3-5 3-8c0-4-4-6-8-3Z"/>',
  edit: '<path d="m14 5 5 5M4 20l4.5-1 12-12-3.5-3.5L5 15.5 4 20Z"/>',
  x: '<path d="m6 6 12 12M18 6 6 18"/>'
};

export function icon(name: keyof typeof paths, extraClass = ''): string {
  return `<svg class="icon ${extraClass}" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="1.7" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true">${paths[name]}</svg>`;
}

export function openForm(
  title: string,
  intro: string,
  fields: string,
  submitLabel: string,
  onSubmit: (data: FormData) => Promise<void>
): void {
  const dialog = document.createElement('dialog');
  dialog.className = 'form-dialog';
  dialog.setAttribute('aria-labelledby', 'dialog-title');
  dialog.innerHTML = `<form class="dialog-form">
    <header class="dialog-heading">
      <div><h2 id="dialog-title">${title}</h2><p>${intro}</p></div>
      <button type="button" class="icon-button" data-close aria-label="Cerrar">${icon('x')}</button>
    </header>
    <div class="dialog-fields">${fields}</div>
    <p class="form-error" role="alert" hidden></p>
    <footer class="dialog-actions">
      <button type="button" class="btn btn-secondary" data-close>Cancelar</button>
      <button type="submit" class="btn btn-primary">${submitLabel}</button>
    </footer>
  </form>`;
  document.body.append(dialog);
  const form = dialog.querySelector('form')!;
  const error = dialog.querySelector<HTMLElement>('.form-error')!;
  const submit = dialog.querySelector<HTMLButtonElement>('[type=submit]')!;
  dialog.querySelectorAll('[data-close]').forEach(button => button.addEventListener('click', () => dialog.close()));
  dialog.addEventListener('close', () => dialog.remove(), { once: true });
  form.addEventListener('submit', async event => {
    event.preventDefault();
    error.hidden = true;
    submit.disabled = true;
    try {
      await onSubmit(new FormData(form));
      dialog.close();
    } catch (failure) {
      error.textContent = errorMessage(failure);
      error.hidden = false;
    } finally {
      submit.disabled = false;
    }
  });
  dialog.showModal();
  form.querySelector<HTMLElement>('input, select, textarea')?.focus();
}
