/// <reference types="vite/client" />
import { getDocument, GlobalWorkerOptions } from 'pdfjs-dist';
import workerUrl from 'pdfjs-dist/build/pdf.worker.min.mjs?url';

GlobalWorkerOptions.workerSrc = workerUrl;

export function openPdf(bytes: ArrayBuffer) {
  const task = getDocument({ data: bytes, useWasm: false, enableXfa: false });
  return {
    dispose: () => { void task.destroy().catch(() => undefined); },
    ready: task.promise.then(document => ({
      pages: document.numPages,
      async draw(index: number, canvas: HTMLCanvasElement, width: number) {
        const page = await document.getPage(index);
        const scale = Math.min(1.6, width / page.getViewport({ scale: 1 }).width);
        const viewport = page.getViewport({ scale });
        const density = Math.min(window.devicePixelRatio || 1, 2, 3000 / Math.max(viewport.width, viewport.height));
        canvas.width = Math.max(1, Math.ceil(viewport.width * density));
        canvas.height = Math.max(1, Math.ceil(viewport.height * density));
        canvas.style.width = `${viewport.width}px`;
        await page.render({ canvas, viewport, transform: [density, 0, 0, density, 0, 0] }).promise;
        page.cleanup();
      }
    }))
  };
}
