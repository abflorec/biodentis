import { escapeHtml } from './shared';

export const inDesktop = Boolean((window as Window & { chrome?: { webview?: unknown } }).chrome?.webview);

export function showDesktopDocument(path: string, name: string, trigger: HTMLElement): void {
  const modal = document.createElement('dialog');
  modal.className = 'radiograph-viewer';
  modal.setAttribute('aria-labelledby', 'desktop-document-title');
  modal.innerHTML = `<header>
    <div><p>Documento clínico</p><h2 id="desktop-document-title">${escapeHtml(name)}</h2></div>
    <button type="button" class="care-button secondary" data-close>Cerrar</button>
  </header>
  <div class="radiograph-toolbar"><a href="${escapeHtml(path)}" download>Guardar archivo</a>
    <div class="desktop-document-navigation" data-navigation hidden>
      <button type="button" class="care-button secondary" data-prev>Anterior</button>
      <span data-page role="status"></span>
      <button type="button" class="care-button secondary" data-next>Siguiente</button>
    </div>
  </div>
  <div class="radiograph-canvas desktop-document"><p role="status" data-status>Abriendo documento…</p></div>`;
  const signal = new AbortController();
  let release: (() => void) | undefined;
  const canvasArea = modal.querySelector<HTMLElement>('.desktop-document')!;
  const status = modal.querySelector<HTMLElement>('[data-status]')!;
  const navigation = modal.querySelector<HTMLElement>('[data-navigation]')!;
  const previous = modal.querySelector<HTMLButtonElement>('[data-prev]')!;
  const next = modal.querySelector<HTMLButtonElement>('[data-next]')!;
  document.body.append(modal);
  modal.querySelector('[data-close]')?.addEventListener('click', () => modal.close());
  modal.addEventListener('close', () => {
    signal.abort();
    release?.();
    modal.remove();
    trigger.focus();
  }, { once: true });
  modal.showModal();

  function failed(): void {
    if (signal.signal.aborted) return;
    navigation.hidden = true;
    status.hidden = false;
    status.textContent = 'No se pudo mostrar el documento. Puedes guardar el archivo para abrirlo con otro visor.';
  }

  async function load(): Promise<void> {
    const response = await fetch(path, { signal: signal.signal });
    if (!response.ok) throw new Error('Documento no disponible');
    const type = response.headers.get('Content-Type');
    if (type === 'image/png' || type === 'image/jpeg') {
      // The browser loads the authenticated image from the same local origin.
      await response.body?.cancel();
      if (signal.signal.aborted) return;
      const image = new Image();
      image.alt = name;
      image.onload = () => { status.hidden = true; };
      image.onerror = failed;
      canvasArea.append(image);
      image.src = path;
      return;
    }
    if (type !== 'application/pdf') throw new Error('Formato no compatible');
    const [bytes, { openPdf }] = await Promise.all([response.arrayBuffer(), import('./desktop-pdf')]);
    if (signal.signal.aborted) return;
    const pdf = openPdf(bytes);
    release = pdf.dispose;
    const document = await pdf.ready;
    if (signal.signal.aborted) return;

    let current = 1;
    const canvas = window.document.createElement('canvas');
    canvas.setAttribute('aria-label', name);
    canvasArea.append(canvas);

    async function render(): Promise<void> {
      previous.disabled = next.disabled = true;
      await document.draw(current, canvas, Math.max(280, canvasArea.clientWidth - 32));
      if (signal.signal.aborted) return;
      status.hidden = true;
      modal.querySelector('[data-page]')!.textContent = `${current} de ${document.pages}`;
      navigation.hidden = document.pages === 1;
      previous.disabled = current === 1;
      next.disabled = current === document.pages;
    }
    previous.onclick = () => { current--; void render().catch(failed); };
    next.onclick = () => { current++; void render().catch(failed); };
    await render();
  }
  void load().catch(failed);
}
