import { api, escapeHtml, errorMessage, notify } from './shared';
import { inDesktop, showDesktopDocument } from './desktop';

interface Radiograph {
  id: number;
  name: string;
  mime: string;
  createdAt: string;
}

export function validateRadiograph(file: File): void {
  if (file.size > 8 * 1024 * 1024) throw new Error('La radiografía supera los 8 MB.');
  if (!['image/jpeg', 'image/png', 'application/pdf'].includes(file.type)) throw new Error('Selecciona una radiografía JPG, PNG o PDF.');
}

export async function attachRadiograph(procedureId: number, file: File): Promise<void> {
  validateRadiograph(file);
  const body = new FormData();
  body.append('file', file);
  await api(`/api/procedures/${procedureId}/radiographs`, { method: 'POST', body });
}

export function bindRadiographs(row: HTMLElement, procedure: { id: number }): void {
  const gallery = row.querySelector<HTMLElement>('[data-radiograph-gallery]')!;
  const form = row.querySelector<HTMLFormElement>('[data-radiograph-form]')!;
  const details = row.querySelector<HTMLDetailsElement>('.care-radiographs')!;
  let loaded = false;
  async function refresh(): Promise<void> {
    gallery.textContent = 'Cargando radiografías…';
    try {
      const items = await api<Radiograph[]>(`/api/procedures/${procedure.id}/radiographs`);
      gallery.innerHTML = items.length ? `<div class="care-radiograph-grid">${items.map(item => `
        <button type="button" class="care-radiograph-card" data-open-radiograph="${item.id}">
          <span class="care-radiograph-preview">
            ${item.mime === 'application/pdf' ? '<span class="care-pdf-preview">PDF<span>Ver documento</span></span>'
              : `<span class="care-pdf-preview">Imagen<span>Abrir original</span></span><img data-thumbnail="/api/radiographs/${item.id}/thumbnail" alt="" loading="lazy" decoding="async">`}
          </span>
          <span class="care-radiograph-caption"><strong>${escapeHtml(item.name)}</strong><small>Ver radiografía ↗</small></span>
        </button>`).join('')}</div>` : '<p class="care-empty">Aún no hay radiografías adjuntas a este procedimiento.</p>';
      gallery.querySelectorAll<HTMLButtonElement>('[data-open-radiograph]').forEach(button => {
        button.onclick = () => {
          const item = items.find(entry => entry.id === Number(button.dataset.openRadiograph))!;
          const src = `/api/radiographs/${item.id}`;
          if (item.mime === 'application/pdf') {
            if (inDesktop) showDesktopDocument(src, item.name, button);
            else window.open(src, '_blank', 'noopener,noreferrer');
            return;
          }
          const modal = document.createElement('dialog');
          modal.className = 'radiograph-viewer';
          const originalLink = inDesktop
            ? `<a href="${src}" download>Guardar archivo</a>`
            : `<a href="${src}" target="_blank" rel="noopener noreferrer">Abrir archivo original ↗</a>`;
          modal.innerHTML = `<header><div><p>Radiografía del tratamiento</p><h2>${escapeHtml(item.name)}</h2></div><button type="button" class="care-button secondary" data-close>Cerrar</button></header>
            <div class="radiograph-toolbar"><label>Ampliación <input type="range" min="100" max="300" step="25" value="100" aria-label="Ampliación de radiografía"></label>${originalLink}</div>
            <div class="radiograph-canvas"><img src="${src}" alt="${escapeHtml(item.name)}"></div>`;
          document.body.append(modal);
          modal.querySelector('[data-close]')!.addEventListener('click', () => modal.close());
          modal.querySelector<HTMLInputElement>('input[type="range"]')?.addEventListener('input', event => {
            const image = modal.querySelector<HTMLImageElement>('.radiograph-canvas img')!;
            image.style.width = `${(event.target as HTMLInputElement).value}%`;
          });
          modal.addEventListener('close', () => { modal.remove(); button.focus(); });
          modal.showModal();
        };
      });
      gallery.querySelectorAll<HTMLImageElement>('[data-thumbnail]').forEach(image => {
        image.addEventListener('load', () => { image.classList.add('is-ready'); }, { once: true });
        image.src = image.dataset.thumbnail!;
      });
      loaded = true;
    } catch (error) {
      gallery.textContent = errorMessage(error);
      const retry = document.createElement('button');
      retry.type = 'button'; retry.className = 'care-text-button'; retry.textContent = 'Volver a cargar';
      retry.onclick = () => { void refresh(); };
      gallery.append(retry);
    }
  }
  details.addEventListener('toggle', () => { if (details.open && !loaded) void refresh(); });
  if (details.open) void refresh();
  form.onsubmit = async event => {
    event.preventDefault();
    const file = new FormData(form).get('file');
    const submit = form.querySelector<HTMLButtonElement>('button[type="submit"]')!;
    const errorBox = form.querySelector('.care-form-error');
    errorBox?.remove();
    submit.disabled = true;
    try {
      if (!(file instanceof File) || !file.size) throw new Error('Selecciona una radiografía.');
      await attachRadiograph(procedure.id, file);
      form.reset();
      notify('Radiografía guardada en el procedimiento.');
      await refresh();
    } catch (error) {
      const message = document.createElement('p');
      message.className = 'care-form-error'; message.setAttribute('role', 'alert');
      message.textContent = errorMessage(error); form.append(message);
    } finally { submit.disabled = false; }
  };
}
