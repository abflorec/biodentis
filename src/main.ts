import './styles.css';
import { renderCash, renderPatientCare } from './care';
import type { Encounter } from './care';
import { installDatePickers } from './datepicker';
import './refresh.css';
import { api, ApiError, dateLabel, errorMessage, escapeHtml, localDate, notify } from './shared';
import { agendaHeading, appointmentList, dateShift, editAppointment, editPatient, emptyState, renderPatientRows } from './registry';
import type { Appointment, Patient } from './registry';
import { icon, openForm } from './ui';

interface Session {
  username: string;
  demo?: boolean;
}

const app = document.querySelector<HTMLElement>('#app')!;
const navItems = [
  ['inicio', 'Inicio', 'home'],
  ['agenda', 'Agenda', 'calendar'],
  ['pacientes', 'Pacientes', 'people'],
  ['caja', 'Caja', 'wallet']
] as const;
let session: Session | undefined;
let agendaDate = localDate();

function brand(): string {
  return `<span class="brand"><img src="/marca.svg" width="36" height="36" alt=""><span>BIODENTIS<small>Gestión odontológica</small></span></span>`;
}

function toothArt(): string {
  return `<div class="tooth-plate" aria-hidden="true">
    <div class="tooth-diagram">${icon('tooth')}</div>
    <span>FIG. 01 / ODONTOLOGÍA</span>
  </div>`;
}

function showLogin(): void {
  document.title = 'Iniciar sesión · BIODENTIS';
  app.innerHTML = `<main class="login-layout">
    <section class="login-intro">
      ${brand()}
      <div class="login-copy">
        <span class="eyebrow">Espacio de trabajo clínico</span>
        <h1>Tu clínica,<br>en orden.</h1>
        <p>Agenda, atenciones y cobros en un solo lugar.</p>
      </div>
      ${toothArt()}
      <span class="local-label"><span></span> Sistema local · Opera sin internet</span>
    </section>
    <section class="login-access">
      <div class="login-card">
        <span class="tile-icon">${icon('lock')}</span>
        <h2>Iniciar sesión</h2>
        <p>Ingresa tu usuario y contraseña para abrir BIODENTIS.</p>
        <form id="login-form">
          <label class="field"><span>Nombre de usuario</span><input name="username" required autocomplete="username" maxlength="64" placeholder="Tu usuario"></label>
          <label class="field"><span>Contraseña</span><input name="password" type="password" required autocomplete="current-password" placeholder="Tu contraseña"></label>
          <p id="login-error" class="form-error" role="alert" hidden></p>
          <button class="btn btn-primary login-button" type="submit">Ingresar ${icon('arrow')}</button>
        </form>
      </div>
    </section>
  </main>`;
  const form = app.querySelector<HTMLFormElement>('#login-form')!;
  const submit = form.querySelector<HTMLButtonElement>('button')!;
  const error = form.querySelector<HTMLElement>('#login-error')!;
  form.addEventListener('submit', async event => {
    event.preventDefault();
    error.hidden = true;
    submit.disabled = true;
    try {
      session = await api<Session>('/api/login', { method: 'POST', body: JSON.stringify(Object.fromEntries(new FormData(form))) });
      await renderRoute();
    } catch (failure) {
      error.textContent = errorMessage(failure);
      error.hidden = false;
    } finally {
      submit.disabled = false;
    }
  });
}

function passwordForm(): void {
  openForm('Cambiar contraseña', 'Elige una contraseña que solo tú conozcas.',
    `<div class="field-stack">
      <label class="field"><span>Contraseña actual</span><input name="currentPassword" type="password" required autocomplete="current-password"></label>
      <label class="field"><span>Nueva contraseña</span><input name="newPassword" type="password" minlength="12" required autocomplete="new-password"></label>
    </div>`, 'Guardar contraseña', async data => {
      await api('/api/password', { method: 'POST', body: JSON.stringify(Object.fromEntries(data)) });
      notify('Contraseña actualizada.');
    });
}

function layout(active: string, activeSession: Session): HTMLElement {
  app.innerHTML = `<div class="shell">
    <a class="skip-link" href="#main-content">Saltar al contenido</a>
    <header class="topbar">
      <div class="topbar-inner">
        <a href="#inicio" class="brand-link">${brand()}</a>
        <nav class="navigation" aria-label="Secciones del sistema">
          ${navItems.map(([key, label, glyph]) => `<a href="#${key}" ${key === active ? 'aria-current="page"' : ''}>${icon(glyph)}<span>${label}</span></a>`).join('')}
        </nav>
        <div class="topbar-account">
          <span class="local-label"><span></span> ${activeSession.demo ? 'Demo local' : 'Operación local'}</span>
          <details class="account-menu">
            <summary><span class="account-avatar">${icon('person')}</span><span>${escapeHtml(activeSession.username)}</span>${icon('next', 'account-chevron')}</summary>
            <div class="account-dropdown">
              <button type="button" id="change-password">${icon('lock')} Cambiar contraseña</button>
              <button type="button" id="logout">${icon('logout')} Cerrar sesión</button>
            </div>
          </details>
        </div>
      </div>
    </header>
    <main class="content" id="main-content" tabindex="-1"></main>
  </div>`;
  app.querySelector('.skip-link')?.addEventListener('click', event => {
    event.preventDefault();
    app.querySelector<HTMLElement>('#main-content')?.focus();
  });
  const account = app.querySelector<HTMLDetailsElement>('.account-menu')!;
  app.querySelector('#change-password')?.addEventListener('click', () => {
    account.open = false;
    passwordForm();
  });
  app.querySelector('#logout')?.addEventListener('click', async () => {
    try {
      await api('/api/logout', { method: 'POST' });
      session = undefined;
      showLogin();
    } catch (failure) {
      notify(errorMessage(failure), 'error');
    }
  });
  return app.querySelector<HTMLElement>('.content')!;
}

function pageHeader(label: string, title: string, description: string, actions = ''): string {
  return `<header class="page-heading"><div><p class="eyebrow">${label}</p><h1>${title}</h1><p>${description}</p></div>
    ${actions ? `<div class="page-actions">${actions}</div>` : ''}</header>`;
}

function button(id: string, text: string, primary = true, glyph: Parameters<typeof icon>[0] = 'plus'): string {
  return `<button type="button" id="${id}" class="btn ${primary ? 'btn-primary' : 'btn-secondary'}">${icon(glyph)} ${text}</button>`;
}

async function showDashboard(container: HTMLElement): Promise<void> {
  const today = localDate();
  const [patients, appointments, encounters] = await Promise.all([
    api<Patient[]>('/api/patients'), api<Appointment[]>('/api/appointments?date=' + today),
    api<Encounter[]>('/api/encounters?date=' + today)
  ]);
  const scheduled = appointments.filter(item => item.status === 'scheduled');
  const cards = [
    ['calendar', 'Citas por atender', scheduled.length],
    ['check', 'Atenciones de hoy', encounters.length],
    ['people', 'Pacientes registrados', patients.length]
  ] as const;

  container.innerHTML = `<section class="dashboard-hero">
    <div><p class="eyebrow">Panel clínico</p>
      <h1>Resumen de la clínica</h1>
      <p>Revisa tu agenda y continúa la atención de tus pacientes.</p>
      <div class="hero-actions">
        <a class="btn btn-primary" href="#agenda">${icon('calendar')} Ver agenda</a>
        ${button('new-patient-home', 'Nuevo paciente', false)}
      </div>
    </div>
    <div class="dashboard-date">
      <span class="eyebrow">Hoy</span>
      <time datetime="${today}"><strong aria-hidden="true">${today.slice(-2)}</strong><span>${escapeHtml(dateLabel(today, true))}</span></time>
    </div>
  </section>
  <section class="metrics" aria-label="Indicadores">
    ${cards.map(([glyph, label, total]) => `<article class="metric panel"><div><p>${label}</p><strong>${total}</strong></div>
      <span class="tile-icon">${icon(glyph)}</span></article>`).join('')}
  </section>
  <section class="panel dashboard-appointments">
    <header class="section-heading"><div><h2>Agenda de hoy</h2><p>Próximas atenciones programadas.</p></div><a href="#agenda" class="text-link">Ver toda la agenda ${icon('next')}</a></header>
    ${scheduled.length ? appointmentList(scheduled, false) : emptyState('Sin citas pendientes', 'Las próximas atenciones de hoy aparecerán aquí.', '<a href="#agenda" class="btn btn-secondary">Abrir agenda</a>')}
  </section>`;
  container.querySelector('#new-patient-home')?.addEventListener('click', () => editPatient(undefined, openPatient));
}

function openPatient(patient: Patient): void {
  window.location.hash = '#paciente/' + patient.id;
}

async function showAgenda(container: HTMLElement): Promise<void> {
  const date = agendaDate;
  const [patients, appointments] = await Promise.all([
    api<Patient[]>('/api/patients'), api<Appointment[]>('/api/appointments?date=' + date)
  ]);
  container.innerHTML = `${pageHeader('Planificación', 'Agenda', 'Organiza tus citas y abre la atención desde el horario.', button('new-appointment', 'Nueva cita'))}
    <section class="panel agenda-panel">
      <header class="section-heading">
        <div><h2>${escapeHtml(agendaHeading(date))}</h2><p>${appointments.length} ${appointments.length === 1 ? 'cita registrada' : 'citas registradas'}</p></div>
        <div class="date-controls">
          <button class="icon-button" id="day-previous" type="button" aria-label="Día anterior">${icon('previous')}</button>
          <label class="field"><span class="sr-only">Fecha de agenda</span><input id="agenda-date" type="date" value="${date}"></label>
          <button class="icon-button" id="day-next" type="button" aria-label="Día siguiente">${icon('next')}</button>
          <button type="button" class="btn btn-secondary" id="day-today">Hoy</button>
        </div>
      </header>
      ${appointments.length ? appointmentList(appointments, true) : emptyState('Sin citas para este día', 'Puedes programar una cita para un paciente registrado.')}
    </section>`;
  const changeDate = (next: string) => {
    if (!next) return;
    agendaDate = next;
    void renderRoute();
  };
  container.querySelector('#day-previous')?.addEventListener('click', () => changeDate(dateShift(date, -1)));
  container.querySelector('#day-next')?.addEventListener('click', () => changeDate(dateShift(date, 1)));
  container.querySelector('#day-today')?.addEventListener('click', () => changeDate(localDate()));
  const input = container.querySelector<HTMLInputElement>('#agenda-date')!;
  input.addEventListener('change', () => changeDate(input.value));
  const appointmentSaved = (appointment: Appointment) => {
    agendaDate = appointment.date;
    void renderRoute();
  };
  container.querySelector('#new-appointment')?.addEventListener('click', () => {
    if (patients.length === 0) {
      editPatient(undefined, patient => editAppointment([patient], date, appointmentSaved, undefined, patient.id));
      return;
    }
    editAppointment(patients, date, appointmentSaved);
  });
  container.querySelectorAll<HTMLButtonElement>('[data-edit-appointment]').forEach(element => {
    element.addEventListener('click', () => {
      const selected = appointments.find(item => item.id === Number(element.dataset.editAppointment));
      if (selected) editAppointment(patients, date, appointmentSaved, selected);
    });
  });
}

async function showPatients(container: HTMLElement): Promise<void> {
  const patients = await api<Patient[]>('/api/patients');
  container.innerHTML = `${pageHeader('Directorio clínico', 'Pacientes', 'Encuentra o registra la ficha de un paciente.', button('new-patient', 'Nuevo paciente'))}
    <section class="panel patients-panel">
      <header class="section-heading"><div><h2>Directorio</h2><p>${patients.length} ${patients.length === 1 ? 'paciente registrado' : 'pacientes registrados'}</p></div>
        <label class="search-field">${icon('search')}<span class="sr-only">Buscar pacientes</span><input type="search" id="patient-search" placeholder="Nombre, historia clínica o teléfono"></label></header>
      <div id="patient-rows"></div>
    </section>`;
  renderPatientRows(container, patients);
  container.querySelector('#new-patient')?.addEventListener('click', () => editPatient(undefined, openPatient));
}

async function showPatient(container: HTMLElement, patientId: number, appointmentId?: number): Promise<void> {
  const patient = await api<Patient>('/api/patients/' + patientId);
  const contact = [patient.phone, patient.email].filter(Boolean).join(' · ');
  const demographics = [patient.age != null && `${patient.age} años`, patient.document && 'Documento ' + patient.document, patient.address].filter(Boolean);
  container.innerHTML = `<a href="#pacientes" class="back-link">${icon('previous')} Pacientes</a>
    ${pageHeader('Historia clínica · ' + escapeHtml(patient.recordNumber), escapeHtml(patient.name), escapeHtml(contact || 'Ficha del paciente'),
      button('edit-patient', 'Editar datos', false, 'edit') + button('patient-appointment', 'Agendar cita', true, 'calendar'))}
    ${demographics.length ? `<div class="patient-details">${demographics.map(item => `<span>${escapeHtml(item)}</span>`).join('')}</div>` : ''}
    <div id="patient-care"></div>`;
  container.querySelector('#edit-patient')?.addEventListener('click', () => editPatient(patient, () => void renderRoute()));
  container.querySelector('#patient-appointment')?.addEventListener('click', () => {
    editAppointment([patient], localDate(), appointment => {
      agendaDate = appointment.date;
      window.location.hash = '#agenda';
    }, undefined, patient.id);
  });
  await renderPatientCare(container.querySelector<HTMLElement>('#patient-care')!, patientId, appointmentId);
}

async function renderRoute(): Promise<void> {
  if (!session) {
    showLogin();
    return;
  }
  const route = window.location.hash.slice(1);
  const patient = route.match(/^paciente\/(\d+)(?:\/cita\/(\d+))?$/);
  const key = patient ? 'pacientes' : navItems.find(item => item[0] === route)?.[0] ?? 'inicio';
  const container = layout(key, session);
  document.title = (patient ? 'Historia clínica' : navItems.find(item => item[0] === key)![1]) + ' · BIODENTIS';
  container.innerHTML = '<p class="loading" role="status">Cargando…</p>';
  try {
    if (patient) await showPatient(container, Number(patient[1]), patient[2] ? Number(patient[2]) : undefined);
    else if (key === 'agenda') await showAgenda(container);
    else if (key === 'pacientes') await showPatients(container);
    else if (key === 'caja') {
      container.innerHTML = `${pageHeader('Gestión de cobros', 'Caja', 'Consulta los cobros, el saldo y el historial de cambios.')}<div id="cash-content"></div>`;
      await renderCash(container.querySelector<HTMLElement>('#cash-content')!);
    } else await showDashboard(container);
  } catch (failure) {
    if (container.isConnected) container.innerHTML = `<div class="panel panel-error" role="alert"><h1>No se pudo abrir esta pantalla</h1><p>${escapeHtml(errorMessage(failure))}</p><a href="#inicio" class="btn btn-secondary">Ir al inicio</a></div>`;
  }
}

window.addEventListener('hashchange', () => { if (session) void renderRoute(); });
window.addEventListener('biodentis:expired', () => {
  session = undefined;
  showLogin();
  notify('Tu sesión terminó. Vuelve a iniciar sesión.', 'error');
});

async function start(): Promise<void> {
  try {
    session = await api<Session>('/api/session');
    await renderRoute();
  } catch (failure) {
    showLogin();
    if (!(failure instanceof ApiError) || failure.status !== 401) notify(errorMessage(failure), 'error');
  }
}

installDatePickers();
void start();
