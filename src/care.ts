import { attachRadiograph, bindRadiographs, validateRadiograph } from './radiographs';
import { api, errorMessage, escapeHtml, localDate, money, notify } from './shared';
import './care.css';

type Values = Record<string, string>;
type ClinicalStatus = 'planned' | 'in_progress' | 'completed';

interface Patient {
  name: string;
  recordNumber: string;
}

export interface Encounter {
  id: number;
  patientId: number;
  appointmentId: number | null;
  date: string;
  questionnaire: Values;
  vitals: Values;
  diagnosis: string;
  prognosis: string;
  extraoral: string;
  intraoral: string;
  notes: string;
  laboratory: string;
  teeth: Values;
}

interface Procedure {
  id: number;
  patientId: number;
  encounterId: number | null;
  name: string;
  tooth: string;
  description: string;
  priceCents: number;
  paidCents: number;
  balanceCents: number;
  clinicalStatus: ClinicalStatus;
  paymentStatus: string;
  consentName: string;
  createdAt: string;
}

interface Payment {
  id: number;
  procedureId: number;
  procedureName: string;
  patientName: string;
  amountCents: number;
  method: string;
  receivedDate: string;
  confirmedAt: string;
  username: string;
  voidedAt: string | null;
  voidedBy: string | null;
  voidReason: string | null;
}

interface Audit {
  id: number;
  action: string;
  entity: string;
  entityId: number;
  username: string;
  at: string;
  detail: string;
}

const questionnaireFields = [
  ['hipertension', 'Hipertensión arterial'],
  ['hemorragias', 'Antecedentes hemorrágicos'],
  ['alergias', 'Antecedentes alérgicos'],
  ['embarazo', 'Embarazo'],
  ['sincope', 'Síncope o desmayo'],
  ['diabetes', 'Diabetes'],
  ['hepaticas', 'Enfermedades hepáticas'],
  ['renal', 'Enfermedad renal'],
  ['cardiacos', 'Antecedentes cardíacos'],
  ['angina', 'Angina de pecho'],
  ['fiebre_reumatica', 'Fiebre reumática'],
  ['otros', 'Otros antecedentes']
] as const;

const examFields = [
  ['diagnosis', 'Diagnóstico bucal'],
  ['prognosis', 'Pronóstico'],
  ['extraoral', 'Examen extraoral (ATM)'],
  ['intraoral', 'Examen intraoral'],
  ['laboratory', 'Laboratorio'],
  ['notes', 'Observaciones']
] as const;

const dentitions = {
  permanent: [
    ['18', '17', '16', '15', '14', '13', '12', '11', '21', '22', '23', '24', '25', '26', '27', '28'],
    ['48', '47', '46', '45', '44', '43', '42', '41', '31', '32', '33', '34', '35', '36', '37', '38']
  ],
  primary: [
    ['55', '54', '53', '52', '51', '61', '62', '63', '64', '65'],
    ['85', '84', '83', '82', '81', '71', '72', '73', '74', '75']
  ]
};

const catalogue = [
  'Profilaxis simple', 'Profilaxis con fluoración', 'Amalgama', 'Resina',
  'Reconstrucción coronaria', 'Tratamiento de conducto anterior',
  'Tratamiento de conducto posterior', 'Perno', 'Corona o puente',
  'Prótesis removible', 'Prótesis total', 'Extracción simple', 'Extracción compleja',
  'Blanqueamiento dental', 'Ortodoncia fija (brackets)', 'Ortodoncia con alineadores'
];

const statusLabels: Record<ClinicalStatus, string> = {
  planned: 'Planificado', in_progress: 'En tratamiento', completed: 'Finalizado'
};

const methods: Record<string, string> = {
  cash: 'Efectivo', transfer: 'Transferencia', card: 'Tarjeta',
  yape: 'Yape', plin: 'Plin', other: 'Otro'
};

const auditLabels: Record<string, string> = {
  'radiograph.attached': 'Radiografía adjuntada',
  'payment.confirmed': 'Pago confirmado', 'payment.voided': 'Cobro anulado',
  'encounter.created': 'Atención creada', 'encounter.updated': 'Atención actualizada',
  'procedure.created': 'Procedimiento creado', 'procedure.status_updated': 'Estado clínico actualizado',
  'consent.attached': 'Consentimiento adjuntado', 'patient.created': 'Paciente registrado',
  'patient.updated': 'Ficha actualizada', 'appointment.created': 'Cita creada',
  'appointment.updated': 'Cita actualizada'
};

function dateLabel(value: string): string {
  const [year, month, day] = value.split('-');
  return year && month && day ? `${day}/${month}/${year}` : '—';
}

function dateTimeLabel(value: string): string {
  const parsed = new Date(value);
  return Number.isNaN(parsed.valueOf()) ? '—' : new Intl.DateTimeFormat('es-PE', {
    timeZone: 'America/Lima', dateStyle: 'short', timeStyle: 'short'
  }).format(parsed);
}

function control<T extends HTMLElement>(parent: ParentNode, selector: string): T {
  return parent.querySelector<T>(selector)!;
}

function formText(data: FormData, key: string): string {
  return String(data.get(key) ?? '').trim();
}

function cents(value: string): number {
  const normalized = value.trim().replace(',', '.');
  if (!/^\d{1,8}(\.\d{1,2})?$/.test(normalized)) return 0;
  const [whole, decimal = ''] = normalized.split('.');
  return Number(whole) * 100 + Number(decimal.padEnd(2, '0'));
}

function post<T>(path: string, data: unknown, method = 'POST'): Promise<T> {
  return api<T>(path, { method, body: JSON.stringify(data) });
}

function bindForm(form: HTMLFormElement, run: (data: FormData) => Promise<void>): void {
  const submit = control<HTMLButtonElement>(form, '[type="submit"]');
  const modal = form.closest('dialog');
  const close = modal?.querySelector<HTMLButtonElement>('.care-close');
  const errorRegion = document.createElement('p');
  errorRegion.className = 'care-form-error';
  errorRegion.setAttribute('role', 'alert');
  const actions = submit.parentElement;
  (actions && actions !== form ? actions : submit).before(errorRegion);
  modal?.addEventListener('cancel', (event) => {
    if (submit.disabled) event.preventDefault();
  });
  form.addEventListener('submit', async (event) => {
    event.preventDefault();
    if (submit.disabled) return;
    const data = new FormData(form);
    submit.disabled = true;
    if (close) close.disabled = true;
    errorRegion.textContent = '';
    try {
      await run(data);
    } catch (error) {
      errorRegion.textContent = errorMessage(error);
    } finally {
      submit.disabled = false;
      if (close) close.disabled = false;
    }
  });
}

function dialog(title: string, contents: string): HTMLDialogElement {
  const element = document.createElement('dialog');
  element.className = 'care-dialog';
  element.setAttribute('aria-label', title);
  element.innerHTML = `<div class="care-dialog-inner">
    <header class="care-dialog-header"><h3>${escapeHtml(title)}</h3>
      <button class="care-close" type="button" aria-label="Cerrar diálogo">×</button></header>
    ${contents}</div>`;
  document.body.append(element);
  control<HTMLButtonElement>(element, '.care-close').onclick = () => element.close();
  element.addEventListener('close', () => element.remove(), { once: true });
  element.showModal();
  return element;
}

function consentSheet(patient: Patient, procedure: Procedure): void {
  document.querySelector('.care-print-sheet')?.remove();
  const sheet = document.createElement('article');
  sheet.className = 'care-print-sheet';
  sheet.innerHTML = `<p class="care-print-brand">BIODENTIS</p>
    <p>Modelo para demostración</p>
    <h1>Consentimiento informado para tratamiento odontológico</h1>
    <dl><dt>Paciente</dt><dd>${escapeHtml(patient.name)}</dd>
      <dt>Historia clínica</dt><dd>${escapeHtml(patient.recordNumber)}</dd>
      <dt>Procedimiento</dt><dd>${escapeHtml(procedure.name)}</dd>
      <dt>Pieza dental</dt><dd>${escapeHtml(procedure.tooth || 'Sin pieza asignada')}</dd>
      <dt>Fecha</dt><dd>${escapeHtml(dateLabel(localDate()))}</dd></dl>
    <p>Descripción del tratamiento</p><p>${escapeHtml(procedure.description || '________________________________________')}</p>
    <p>Alternativas, beneficios y riesgos explicados durante la consulta</p><p class="care-print-space"></p>
    <p>He recibido la explicación sobre el tratamiento y he tenido oportunidad de hacer preguntas.</p>
    <div class="care-signatures"><p>Firma del paciente o representante</p><p>Firma del odontólogo</p></div>`;
  document.body.append(sheet);
  window.addEventListener('afterprint', () => sheet.remove(), { once: true });
  window.print();
}

function bindOdontogram(section: HTMLElement, teeth: Values): () => void {
  const area = control<HTMLElement>(section, '[data-teeth]');
  const note = control<HTMLInputElement>(section, '[name="toothFinding"]');
  const selectedLabel = control<HTMLElement>(section, '[data-tooth-label]');
  const apply = control<HTMLButtonElement>(section, '[data-apply-tooth]');
  let selected = '';
  let dentition: keyof typeof dentitions = 'permanent';

  function commitFinding(): void {
    if (!selected) return;
    const finding = note.value.trim();
    if (finding) teeth[selected] = finding;
    else delete teeth[selected];
  }

  function draw(): void {
    area.innerHTML = dentitions[dentition].map((row, index) => `
      <div class="care-tooth-row" aria-label="${index ? 'Arcada inferior' : 'Arcada superior'}">
        ${row.map((tooth, toothIndex) => `
          <button type="button" class="care-tooth${teeth[tooth] ? ' has-finding' : ''}${toothIndex === row.length / 2 ? ' second-quadrant' : ''}"
            aria-label="Pieza ${tooth}${teeth[tooth] ? ', con hallazgo' : ''}" aria-pressed="${selected === tooth}" data-tooth="${tooth}">
            <span class="care-tooth-mark" aria-hidden="true"></span><span>${tooth}</span>
          </button>`).join('')}
      </div>`).join('');
    area.querySelectorAll<HTMLButtonElement>('[data-tooth]').forEach((button) => {
      button.onclick = () => {
        commitFinding();
        selected = button.dataset.tooth!;
        selectedLabel.textContent = `Hallazgo en la pieza ${selected}`;
        note.disabled = false;
        apply.disabled = false;
        note.value = teeth[selected] || '';
        draw();
        note.focus();
      };
    });
  }

  section.querySelectorAll<HTMLButtonElement>('[data-dentition]').forEach((button) => {
    button.onclick = () => {
      commitFinding();
      dentition = button.dataset.dentition as keyof typeof dentitions;
      section.querySelectorAll<HTMLButtonElement>('[data-dentition]').forEach((item) => {
        item.setAttribute('aria-pressed', String(item === button));
      });
      selected = '';
      note.value = '';
      note.disabled = true;
      apply.disabled = true;
      selectedLabel.textContent = 'Selecciona una pieza';
      draw();
    };
  });
  apply.onclick = () => {
    if (!selected) return;
    commitFinding();
    draw();
    notify(teeth[selected] ? `Hallazgo anotado en la pieza ${selected}. Guarda la atención para conservarlo.` : `Hallazgo retirado de la pieza ${selected}. Guarda la atención para conservar el cambio.`);
  };
  note.addEventListener('keydown', (event) => {
    if (event.key === 'Enter') {
      event.preventDefault();
      apply.click();
    }
  });
  draw();
  return commitFinding;
}

function encounterForm(encounter: Encounter | undefined, forAppointment: boolean): string {
  return `<form class="care-form" data-encounter>
    ${forAppointment ? '<p class="care-context">Atención vinculada a la cita seleccionada.</p>' : ''}
    <label>Fecha de atención<input type="date" name="date" value="${escapeHtml(encounter?.date || localDate())}" required></label>
    <details class="care-details">
      <summary>Antecedentes del paciente <span class="care-summary-hint">Selecciona solo los que presenta</span></summary>
      <p class="care-questionnaire-help">Todos empiezan en No. Cambia a Sí los antecedentes que el paciente indique.</p>
      <div class="care-fields care-questionnaire">
        ${questionnaireFields.map(([key, label]) => `
          <label>${label}<select name="q_${key}">
            <option value="no"${encounter?.questionnaire?.[key] !== 'yes' ? ' selected' : ''}>No</option>
            <option value="yes"${encounter?.questionnaire?.[key] === 'yes' ? ' selected' : ''}>Sí</option>
          </select></label>`).join('')}
      </div>
    </details>
    <div class="care-fields care-exam">
      ${examFields.map(([key, label]) => `<label>${label}<textarea name="${key}" rows="2" maxlength="1800">${escapeHtml(encounter?.[key] || '')}</textarea></label>`).join('')}
    </div>
    <section class="care-odontogram" aria-label="Odontograma interactivo">
      <div class="care-odonto-heading">
        <div><p class="care-kicker">Piezas FDI</p><h3>Odontograma</h3></div>
        <div class="care-segments" aria-label="Dentición">
          <button type="button" data-dentition="permanent" aria-pressed="true">Permanente</button>
          <button type="button" data-dentition="primary" aria-pressed="false">Temporal</button>
        </div>
      </div>
      <div class="care-tooth-grid" data-teeth></div>
      <div class="care-finding">
        <label><span data-tooth-label>Selecciona una pieza</span><input name="toothFinding" maxlength="220" placeholder="Describe el hallazgo" disabled></label>
        <button class="care-button secondary" data-apply-tooth type="button" disabled>Aplicar hallazgo</button>
      </div>
    </section>
    <div class="care-actions"><button class="care-button" type="submit">Guardar atención</button></div>
  </form>`;
}

function paymentRows(payments: Payment[], includePatient: boolean): string {
  if (!payments.length) return '<p class="care-empty">Aún no hay cobros registrados.</p>';
  return `<div class="care-table-wrap"><table class="care-table" role="table"><thead role="rowgroup"><tr role="row">
    ${includePatient ? '<th scope="col">Paciente y procedimiento</th>' : ''}
    <th scope="col">Recibido</th><th scope="col">Confirmación</th><th scope="col">Importe</th><th scope="col">Estado</th><th scope="col"><span class="care-sr">Acciones</span></th>
    </tr></thead><tbody role="rowgroup">${payments.map((payment) => `
      <tr role="row">${includePatient ? `<td role="cell" class="payment-patient"><strong>${escapeHtml(payment.patientName)}</strong><small>${escapeHtml(payment.procedureName)}</small></td>` : ''}
        <td role="cell"><span class="payment-mobile-label" aria-hidden="true">Recibido</span>${escapeHtml(dateLabel(payment.receivedDate))}<small>${escapeHtml(methods[payment.method] || payment.method)}</small></td>
        <td role="cell"><span class="payment-mobile-label" aria-hidden="true">Confirmación</span>${escapeHtml(dateTimeLabel(payment.confirmedAt))}<small>${escapeHtml(payment.username)}</small></td>
        <td role="cell"><span class="payment-mobile-label" aria-hidden="true">Importe</span><strong>${escapeHtml(money(payment.amountCents))}</strong><small>Movimiento #${payment.id}</small></td>
        <td role="cell"><span class="payment-mobile-label" aria-hidden="true">Estado</span>${payment.voidedAt
          ? `<span class="care-pill overdue">Anulado</span><small>${escapeHtml(dateTimeLabel(payment.voidedAt))} · ${escapeHtml(payment.voidedBy)}<br>${escapeHtml(payment.voidReason)}</small>`
          : '<span class="care-pill done">Vigente</span>'}</td>
        <td role="cell" class="payment-actions">${payment.voidedAt ? '' : `<button type="button" class="care-text-button" data-void-payment="${payment.id}">Anular</button>`}</td>
      </tr>`).join('')}</tbody></table></div>`;
}

function bindVoidButtons(container: HTMLElement, after: () => Promise<void>): void {
  container.querySelectorAll<HTMLButtonElement>('[data-void-payment]').forEach((button) => {
    button.onclick = () => {
      const id = Number(button.dataset.voidPayment);
      const panel = dialog('Anular cobro', `<form class="care-form">
        <p>El movimiento #${id} quedará en la auditoría y su importe volverá al saldo del procedimiento.</p>
        <label>Motivo de anulación<textarea name="reason" rows="3" maxlength="300" required autofocus></textarea></label>
        <div class="care-actions"><button type="submit" class="care-button danger">Confirmar anulación</button></div></form>`);
      bindForm(control<HTMLFormElement>(panel, 'form'), async (data) => {
        const reason = formText(data, 'reason');
        if (!reason) throw new Error('Indica el motivo de la anulación.');
        await post(`/api/payments/${id}/void`, { reason });
        panel.close();
        notify('Cobro anulado.');
        await after();
      });
    };
  });
}

function pay(procedure: Procedure, settle: boolean, after: () => Promise<void>): void {
  const amount = (procedure.balanceCents / 100).toFixed(2);
  const panel = dialog(settle ? 'Saldar pendiente' : 'Registrar abono', `<form class="care-form">
    <div class="care-payment-summary"><span>${escapeHtml(procedure.name)}</span><strong>${escapeHtml(money(procedure.balanceCents))} de saldo</strong></div>
    <div class="care-fields">
      <label>Importe recibido (S/)<input name="amount" inputmode="decimal" placeholder="0.00" required ${settle ? `value="${amount}" readonly` : 'autofocus'}></label>
      <label>Medio de pago<select name="method">${Object.entries(methods).map(([key, label]) => `<option value="${key}">${label}</option>`).join('')}</select></label>
      <label>Fecha de recepción<input type="date" name="receivedDate" value="${localDate()}" required></label>
    </div>
    <label class="care-check"><input type="checkbox" required> Confirmo que recibí el importe indicado.</label>
    <div class="care-actions"><button type="submit" class="care-button">Confirmar cobro</button></div></form>`);
  let prior = '';
  let requestId = '';
  bindForm(control<HTMLFormElement>(panel, 'form'), async (data) => {
    const amountCents = cents(formText(data, 'amount'));
    if (amountCents <= 0 || amountCents > procedure.balanceCents) throw new Error('Ingresa un importe mayor a cero y dentro del saldo.');
    const body = { amountCents, method: formText(data, 'method'), receivedDate: formText(data, 'receivedDate'), settle };
    const fingerprint = JSON.stringify(body);
    if (fingerprint !== prior) requestId = crypto.randomUUID();
    prior = fingerprint;
    await post(`/api/procedures/${procedure.id}/payments`, { ...body, requestId });
    panel.close();
    notify(settle ? 'Pago registrado. El saldo quedó en cero.' : 'Abono registrado.');
    await after();
  });
}

function procedureMarkup(procedure: Procedure): string {
  const status = procedure.balanceCents === 0 ? 'Pagado' : procedure.paidCents > 0 ? 'Parcial' : 'Pendiente';
  const tone = procedure.balanceCents === 0 ? 'done' : procedure.paidCents > 0 ? 'partial' : 'neutral';
  return `<article class="care-procedure" data-procedure="${procedure.id}">
    <div class="care-procedure-head"><div>
      <h3>${escapeHtml(procedure.name)}</h3><p>${escapeHtml(procedure.tooth ? `Pieza ${procedure.tooth}` : 'Sin pieza asignada')}${procedure.description ? ` · ${escapeHtml(procedure.description)}` : ''}</p>
    </div><span class="care-pill ${tone}">${status}</span></div>
    <dl class="care-balances"><div><dt>Precio</dt><dd>${escapeHtml(money(procedure.priceCents))}</dd></div>
      <div><dt>Abonado</dt><dd>${escapeHtml(money(procedure.paidCents))}</dd></div>
      <div><dt>Saldo</dt><dd>${escapeHtml(money(procedure.balanceCents))}</dd></div></dl>
    <div class="care-procedure-actions">
      <label>Estado clínico<select data-clinical-status>
        ${Object.entries(statusLabels).map(([key, label]) => `<option value="${key}"${procedure.clinicalStatus === key ? ' selected' : ''}>${label}</option>`).join('')}
      </select></label>
      ${procedure.balanceCents > 0 ? '<div class="care-actions"><button type="button" class="care-button secondary" data-abono>Registrar abono</button><button type="button" class="care-button" data-settle>Saldar pendiente</button></div>' : ''}
    </div>
    <details class="care-details care-radiographs" ${/ortodon|bracket|alineador/i.test(procedure.name) ? 'open' : ''}>
      <summary>Radiografías <span class="care-summary-hint">Imágenes del tratamiento</span></summary>
      <div class="care-radiograph-body">
        <div data-radiograph-gallery aria-live="polite"></div>
        <form class="care-form" data-radiograph-form>
          <label>Adjuntar radiografía<input type="file" name="file" accept="image/png,image/jpeg,application/pdf" required></label>
          <div class="care-upload-footer"><small>JPG, PNG o PDF · máximo 8 MB.</small><button class="care-button secondary" type="submit">Guardar radiografía</button></div>
        </form>
      </div>
    </details>
    <details class="care-details care-history">
      <summary>Movimientos del procedimiento</summary><div data-payment-history></div>
    </details>
    <details class="care-details">
      <summary>Consentimiento · ${procedure.consentName ? 'Firmado' : 'Pendiente'}</summary>
      <div class="care-consent">
        <button type="button" class="care-button secondary" data-print>Imprimir modelo</button>
        ${procedure.consentName ? `<a class="care-file" href="/api/procedures/${procedure.id}/consent" target="_blank" rel="noopener noreferrer">Abrir ${escapeHtml(procedure.consentName)}</a>` : '<span class="care-muted">Sin documento firmado</span>'}
        <form class="care-attach"><label>Adjuntar documento firmado<input type="file" name="file" accept="application/pdf,image/jpeg,image/png" required></label>
          <button type="submit" class="care-button secondary">Guardar documento</button></form>
      </div>
    </details>
  </article>`;
}

export async function renderPatientCare(container: HTMLElement, patientId: number, appointmentId?: number): Promise<void> {
  const patientPath = `/api/patients/${patientId}`;
  const encounterPath = `${patientPath}/encounters`;
  const procedurePath = `${patientPath}/procedures`;
  const [patient, initialEncounters, initialProcedures] = await Promise.all([
    api<Patient>(patientPath), api<Encounter[]>(encounterPath), api<Procedure[]>(procedurePath)
  ]);
  let encounters = initialEncounters || [];
  let procedures = initialProcedures || [];
  let encounter = appointmentId ? encounters.find((item) => item.appointmentId === appointmentId) : encounters[0];
  let relatedAppointment = appointmentId ?? encounter?.appointmentId ?? null;

  container.innerHTML = `<div class="care-navigation" aria-label="Secciones de la ficha">
    <button type="button" data-care-view="clinical" aria-pressed="true">Atención clínica <span>01</span></button>
    <button type="button" data-care-view="treatment" aria-pressed="false">Tratamientos y pagos <span>02</span></button>
  </div><div class="care-layout">
    <section class="care-panel" data-care-panel="clinical" aria-labelledby="care-encounters-title">
      <div class="care-panel-head">
        <div><p class="care-kicker">Ficha clínica</p><h2 id="care-encounters-title">Atenciones</h2></div>
        <button class="care-button secondary" type="button" data-new-encounter>Nueva atención</button>
      </div>
      <div data-encounter-history></div><div data-encounter-editor></div>
    </section>
    <section class="care-panel" data-care-panel="treatment" hidden aria-labelledby="care-procedures-title">
      <div class="care-panel-head"><div><p class="care-kicker">Plan y caja</p><h2 id="care-procedures-title">Procedimientos</h2></div></div>
      <p class="care-muted">Cada procedimiento conserva su propio precio, estado clínico y saldo.</p>
      <div class="care-plan-overview" data-plan-overview></div>
      <details class="care-details care-add-procedure" open>
        <summary>Agregar procedimiento</summary>
        <form class="care-form">
          <div class="care-fields">
            <label>Procedimiento<select name="service" required><option value="">Selecciona un procedimiento</option>${catalogue.map((item) => `<option>${item}</option>`).join('')}<option value="other">Otro procedimiento</option></select></label>
            <label data-custom-procedure hidden>Nombre del procedimiento<input name="custom" maxlength="120"></label>
            <label>Pieza dental<input name="tooth" inputmode="numeric" pattern="[1-8][1-8]" placeholder="16" maxlength="2"></label>
            <label>Precio (S/)<input name="price" inputmode="decimal" placeholder="300.00" required></label>
          </div>
          <label>Descripción y sesiones<textarea name="description" rows="2" maxlength="1200"></textarea></label>
          <div class="care-radiograph-prompt" data-new-radiograph hidden>
            <div><span class="care-kicker">Estudios del tratamiento</span><h3>Radiografías</h3>
              <p>Adjunta una imagen inicial o agrégala después. Se guardará en este procedimiento.</p></div>
            <label>Radiografía inicial · opcional<input type="file" name="radiograph" accept="image/png,image/jpeg,application/pdf"></label>
            <small>JPG, PNG o PDF · hasta 8 MB por archivo.</small>
          </div>
          <p class="care-muted" data-related-encounter></p>
          <div class="care-actions"><button type="submit" class="care-button">Guardar procedimiento</button></div>
        </form>
      </details>
      <div data-procedures></div>
    </section>
  </div>`;

  container.querySelectorAll<HTMLButtonElement>('[data-care-view]').forEach(button => {
    button.onclick = () => {
      container.querySelectorAll<HTMLButtonElement>('[data-care-view]').forEach(item => item.setAttribute('aria-pressed', String(item === button)));
      container.querySelectorAll<HTMLElement>('[data-care-panel]').forEach(panel => { panel.hidden = panel.dataset.carePanel !== button.dataset.careView; });
    };
  });

  function showEncounter(): void {
    const history = control<HTMLElement>(container, '[data-encounter-history]');
    const editor = control<HTMLElement>(container, '[data-encounter-editor]');
    history.innerHTML = encounters.length ? `<label class="care-history-select">Atenciones registradas
      <select><option value="">Nueva atención</option>
        ${encounters.map((item) => `<option value="${item.id}"${item.id === encounter?.id ? ' selected' : ''}>${escapeHtml(dateLabel(item.date))} · Atención #${item.id}</option>`).join('')}
      </select></label>` : '';
    const selection = history.querySelector('select');
    if (selection) selection.onchange = () => {
      encounter = encounters.find((item) => item.id === Number(selection.value));
      relatedAppointment = encounter?.appointmentId ?? null;
      showEncounter();
    };

    editor.innerHTML = encounterForm(encounter, relatedAppointment !== null);
    const form = control<HTMLFormElement>(editor, '[data-encounter]');
    const teeth = { ...(encounter?.teeth || {}) };
    const commitFinding = bindOdontogram(control<HTMLElement>(form, '.care-odontogram'), teeth);
    control<HTMLElement>(container, '[data-related-encounter]').textContent = encounter
      ? `Se vinculará a la atención del ${dateLabel(encounter.date)}.`
      : 'Se guardará en el plan del paciente.';
    bindForm(form, async (data) => {
      commitFinding();
      const questionnaire = Object.fromEntries(questionnaireFields
        .map(([key]) => [key, formText(data, `q_${key}`)]).filter(([, value]) => value));
      const vitals = encounter?.vitals ?? {};
      const payload = {
        appointmentId: relatedAppointment, date: formText(data, 'date'), questionnaire, vitals, teeth,
        ...Object.fromEntries(examFields.map(([key]) => [key, formText(data, key)]))
      };
      const updated = await post<Encounter>(encounter ? `/api/encounters/${encounter.id}` : encounterPath, payload, encounter ? 'PUT' : 'POST');
      encounters = [updated, ...encounters.filter((item) => item.id !== updated.id)]
        .sort((first, second) => second.date.localeCompare(first.date) || second.id - first.id);
      encounter = updated;
      relatedAppointment = updated.appointmentId;
      showEncounter();
      notify('Atención guardada.');
    });
  }

  async function reloadProcedures(): Promise<void> {
    procedures = await api<Procedure[]>(procedurePath) || [];
    showProcedures();
  }

  function showProcedures(): void {
    const total = procedures.reduce((sum, item) => sum + item.priceCents, 0);
    const paid = procedures.reduce((sum, item) => sum + item.paidCents, 0);
    control<HTMLElement>(container, '[data-plan-overview]').innerHTML = `<div><span>Plan de tratamiento</span><strong>${procedures.length} ${procedures.length === 1 ? 'procedimiento' : 'procedimientos'}</strong></div><div><span>Presupuesto</span><strong>${escapeHtml(money(total))}</strong></div><div><span>Abonado</span><strong>${escapeHtml(money(paid))}</strong></div><div><span>Saldo pendiente</span><strong>${escapeHtml(money(total - paid))}</strong></div>`;
    const list = control<HTMLElement>(container, '[data-procedures]');
    list.innerHTML = procedures.length
      ? procedures.map(procedureMarkup).join('')
      : '<p class="care-empty">Aún no hay procedimientos en el plan.</p>';
    for (const procedure of procedures) {
      const row = control<HTMLElement>(list, `[data-procedure="${procedure.id}"]`);
      bindRadiographs(row, procedure);
      const status = control<HTMLSelectElement>(row, '[data-clinical-status]');
      status.onchange = async () => {
        status.disabled = true;
        try {
          const updated = await post<Procedure>(`/api/procedures/${procedure.id}/status`, { clinicalStatus: status.value }, 'PUT');
          procedure.clinicalStatus = updated.clinicalStatus;
          notify('Estado clínico actualizado.');
        } catch (error) {
          status.value = procedure.clinicalStatus;
          notify(errorMessage(error), 'error');
        } finally {
          status.disabled = false;
        }
      };
      row.querySelector<HTMLButtonElement>('[data-abono]')?.addEventListener('click', () => pay(procedure, false, reloadProcedures));
      row.querySelector<HTMLButtonElement>('[data-settle]')?.addEventListener('click', () => pay(procedure, true, reloadProcedures));
      control<HTMLButtonElement>(row, '[data-print]').onclick = () => consentSheet(patient, procedure);
      const attachment = control<HTMLFormElement>(row, '.care-attach');
      bindForm(attachment, async (data) => {
        await api<Procedure>(`/api/procedures/${procedure.id}/consent`, { method: 'POST', body: data });
        notify('Documento de consentimiento guardado.');
        await reloadProcedures();
      });
      const details = control<HTMLDetailsElement>(row, '.care-history');
      const history = control<HTMLElement>(row, '[data-payment-history]');
      details.addEventListener('toggle', async () => {
        if (!details.open) return;
        history.textContent = 'Cargando movimientos…';
        try {
          history.innerHTML = paymentRows(await api<Payment[]>(`/api/payments?procedureId=${procedure.id}`) || [], false);
          bindVoidButtons(history, reloadProcedures);
        } catch (error) {
          history.textContent = errorMessage(error);
        }
      });
    }
  }

  control<HTMLButtonElement>(container, '[data-new-encounter]').onclick = () => {
    encounter = undefined;
    relatedAppointment = null;
    showEncounter();
    control<HTMLInputElement>(container, '[data-encounter] [name="date"]').focus();
  };

  const add = control<HTMLFormElement>(container, '.care-add-procedure form');
  const service = control<HTMLSelectElement>(add, '[name="service"]');
  service.onchange = () => {
    const custom = control<HTMLInputElement>(add, '[name="custom"]');
    control<HTMLElement>(add, '[data-custom-procedure]').hidden = service.value !== 'other';
    custom.required = service.value === 'other';
    control<HTMLElement>(add, '[data-new-radiograph]').hidden = !service.value;
  };
  bindForm(add, async (data) => {
    const priceCents = cents(formText(data, 'price'));
    if (!priceCents) throw new Error('Ingresa un precio mayor a cero, con hasta dos decimales.');
    const name = service.value === 'other' ? formText(data, 'custom') : service.value;
    if (!name) throw new Error('Ingresa el nombre del procedimiento.');
    const file = data.get('radiograph');
    if (file instanceof File && file.size) validateRadiograph(file);
    const created = await post<Procedure>(procedurePath, {
      name, tooth: formText(data, 'tooth'), description: formText(data, 'description'),
      priceCents, encounterId: encounter?.id ?? null
    });
    add.reset();
    service.dispatchEvent(new Event('change'));
    let uploadError = '';
    if (file instanceof File && file.size) {
      try { await attachRadiograph(created.id, file); }
      catch (error) { uploadError = errorMessage(error); }
    }
    await reloadProcedures();
    const createdRow = container.querySelector<HTMLElement>(`[data-procedure="${created.id}"]`);
    createdRow?.scrollIntoView({ behavior: 'smooth', block: 'start' });
    if (uploadError) {
      const message = document.createElement('p');
      message.className = 'care-form-error';
      message.textContent = `El procedimiento está guardado, pero falta adjuntar la radiografía: ${uploadError} Puedes volver a adjuntarla aquí.`;
      createdRow?.prepend(message);
      notify('Procedimiento guardado. Revisa el archivo de radiografía.', 'error');
    } else notify('Procedimiento agregado al plan.');
  });
  showEncounter();
  showProcedures();
}

export async function renderCash(container: HTMLElement): Promise<void> {
  const [paymentData, auditData] = await Promise.all([api<Payment[]>('/api/payments'), api<Audit[]>('/api/audit')]);
  const payments = paymentData || [];
  const audits = auditData || [];
  const today = localDate();
  const collected = payments.filter((payment) => payment.receivedDate === today && !payment.voidedAt);
  const total = collected.reduce((sum, payment) => sum + payment.amountCents, 0);
  container.innerHTML = `<div class="care-cash-layout">
    <section class="care-cash-metrics" aria-label="Resumen de caja">
      <div><span>Cobrado hoy</span><strong>${escapeHtml(money(total))}</strong><small>Fecha de recepción · ${escapeHtml(dateLabel(today))}</small></div>
      <div><span>Movimientos del día</span><strong>${collected.length}</strong><small>Cobros vigentes</small></div>
    </section>
    <section class="care-panel"><div class="care-panel-head"><div><p class="care-kicker">Caja</p><h2>Movimientos</h2></div></div>
      ${paymentRows(payments, true)}
    </section>
    <section class="care-panel"><div class="care-panel-head"><div><p class="care-kicker">Registro de cambios</p><h2>Pista de auditoría</h2></div></div>
      ${audits.length ? `<ol class="care-audit">${audits.map((audit) => `<li>
        <div><strong>${escapeHtml(auditLabels[audit.action] || audit.action.replaceAll('_', ' '))}</strong>
          <small>${escapeHtml(audit.username)} · ${escapeHtml(dateTimeLabel(audit.at))}</small></div>
        <p>${escapeHtml(audit.detail)}</p></li>`).join('')}</ol>` : '<p class="care-empty">Aún no hay actividad registrada.</p>'}
    </section>
  </div>`;
  bindVoidButtons(container, () => renderCash(container));
}

