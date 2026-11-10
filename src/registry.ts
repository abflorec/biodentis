import { api, dateLabel, escapeHtml, localDate, notify } from './shared';
import { icon, openForm } from './ui';

export interface Patient {
  id: number;
  recordNumber: string;
  name: string;
  document: string;
  birthDate: string;
  age: number | null;
  sex: string;
  phone: string;
  email: string;
  address: string;
  occupation: string;
  maritalStatus: string;
  other: string;
}

export interface Appointment {
  id: number;
  patientId: number;
  patientName: string;
  date: string;
  time: string;
  duration: number;
  reason: string;
  status: 'scheduled' | 'completed' | 'canceled';
}

const patientFields = [
  ['name', 'Nombre completo', 'text'],
  ['document', 'Documento de identidad', 'text'],
  ['age', 'Edad (años)', 'number'],
  ['phone', 'Teléfono', 'tel'],
  ['email', 'Correo electrónico', 'email'],
  ['address', 'Domicilio', 'text'],
  ['occupation', 'Ocupación', 'text'],
  ['maritalStatus', 'Estado civil', 'text'],
  ['other', 'Otros datos', 'text']
] as const;

export function editPatient(patient: Patient | undefined, saved: (patient: Patient) => void): void {
  const inputs = patientFields.map(([key, label, type]) => {
    const rules = key === 'name' ? 'required maxlength="120" autocomplete="name"' :
      key === 'age' ? 'min="0" max="130" step="1" inputmode="numeric" placeholder="Ej. 28"' : 'maxlength="200"';
    return `<label class="field ${key === 'name' || key === 'other' ? 'field-wide' : ''}">
      <span>${label}${key === 'name' ? ' *' : ''}</span>
      <input name="${key}" type="${type}" value="${escapeHtml(patient?.[key])}" ${rules}>
    </label>`;
  });
  inputs.splice(3, 0, `<label class="field"><span>Sexo</span><select name="sex">
    <option value="">Sin registrar</option>
    ${[['F', 'Femenino'], ['M', 'Masculino'], ['Otro', 'Otro']].map(([value, label]) =>
      `<option value="${value}" ${patient?.sex === value ? 'selected' : ''}>${label}</option>`).join('')}
  </select></label>`);
  openForm(patient ? 'Editar datos del paciente' : 'Nuevo paciente',
    patient ? 'Actualiza los datos de identificación y contacto.' : 'Se asignará un número de historia clínica automáticamente.',
    `<div class="field-grid">${inputs.join('')}</div>`, 'Guardar paciente', async data => {
      const age = String(data.get('age') ?? '').trim();
      const body = { ...Object.fromEntries(data), age: age === '' ? null : Number(age) };
      const result = await api<Patient>(patient ? `/api/patients/${patient.id}` : '/api/patients', {
        method: patient ? 'PUT' : 'POST', body: JSON.stringify(body)
      });
      notify(patient ? 'Datos del paciente actualizados.' : 'Paciente registrado.');
      saved(result);
    });
}

export function editAppointment(
  patients: Patient[],
  date: string,
  saved: (appointment: Appointment) => void,
  appointment?: Appointment,
  selectedPatient?: number
): void {
  const patient = appointment?.patientId ?? selectedPatient;
  const options = patients.map(person =>
    `<option value="${person.id}" ${person.id === patient ? 'selected' : ''}>${escapeHtml(person.name)} · ${escapeHtml(person.recordNumber)}</option>`
  ).join('');
  const statuses: [string, string][] = [['scheduled', 'Programada'], ['completed', 'Atendida'], ['canceled', 'Cancelada']];
  openForm(appointment ? 'Editar cita' : 'Nueva cita',
    'Agenda un horario para la atención odontológica.',
    `<div class="field-grid">
      <label class="field field-wide"><span>Paciente *</span><select name="patientId" required>
        <option value="">Seleccionar paciente</option>${options}</select></label>
      <label class="field"><span>Fecha *</span><input name="date" type="date" required value="${escapeHtml(appointment?.date ?? date)}"></label>
      <label class="field"><span>Hora *</span><input name="time" type="time" required value="${escapeHtml(appointment?.time ?? '09:00')}"></label>
      <label class="field"><span>Duración en minutos *</span><input name="duration" type="number" required min="5" max="480" value="${appointment?.duration ?? 30}"></label>
      ${appointment ? `<label class="field"><span>Estado</span><select name="status">${statuses.map(([value, label]) => `<option value="${value}" ${appointment.status === value ? 'selected' : ''}>${label}</option>`).join('')}</select></label>` : ''}
      <label class="field field-wide"><span>Motivo</span><input name="reason" maxlength="200" placeholder="Ej. Evaluación odontológica" value="${escapeHtml(appointment?.reason)}"></label>
    </div>`, 'Guardar cita', async data => {
      const result = await api<Appointment>(appointment ? `/api/appointments/${appointment.id}` : '/api/appointments', {
        method: appointment ? 'PUT' : 'POST',
        body: JSON.stringify({
          patientId: Number(data.get('patientId')), date: data.get('date'), time: data.get('time'),
          duration: Number(data.get('duration')), reason: data.get('reason'), status: data.get('status') ?? 'scheduled'
        })
      });
      notify(appointment ? 'Cita actualizada.' : 'Cita agendada.');
      saved(result);
    });
}

export function statusLabel(status: Appointment['status']): string {
  return status === 'completed' ? 'Atendida' : status === 'canceled' ? 'Cancelada' : 'Programada';
}

export function appointmentList(appointments: Appointment[], editable: boolean): string {
  return `<div class="appointment-list">${appointments.map(item => `<div class="appointment-row">
    <div class="appointment-time"><strong>${escapeHtml(item.time)}</strong><small>${item.duration} min</small></div>
    <div class="row-main"><a href="#paciente/${item.patientId}${item.status === 'canceled' ? '' : '/cita/' + item.id}">${escapeHtml(item.patientName)}</a><small>${escapeHtml(item.reason || 'Consulta odontológica')}</small></div>
    <span class="pill pill-${item.status}">${statusLabel(item.status)}</span>
    ${editable ? `<button class="icon-button" type="button" data-edit-appointment="${item.id}" aria-label="Editar cita de ${escapeHtml(item.patientName)}">${icon('edit')}</button>` : `<a class="icon-button" href="#paciente/${item.patientId}${item.status === 'canceled' ? '' : '/cita/' + item.id}" aria-label="Abrir ficha de ${escapeHtml(item.patientName)}">${icon('next')}</a>`}
  </div>`).join('')}</div>`;
}

export function dateShift(date: string, days: number): string {
  const instant = new Date(date + 'T12:00:00Z');
  instant.setUTCDate(instant.getUTCDate() + days);
  return instant.toISOString().slice(0, 10);
}

export function emptyState(title: string, message: string, action = ''): string {
  return `<div class="empty-state"><span class="empty-icon">${icon('calendar')}</span><strong>${title}</strong><p>${message}</p>${action}</div>`;
}

export function renderPatientRows(container: HTMLElement, patients: Patient[]): void {
  const input = container.querySelector<HTMLInputElement>('#patient-search')!;
  const rows = container.querySelector<HTMLElement>('#patient-rows')!;
  const normalize = (text: string) => text.toLocaleLowerCase('es-PE').normalize('NFD').replace(/\p{M}/gu, '');
  const draw = () => {
    const query = normalize(input.value.trim());
    const matches = patients.filter(patient => normalize([patient.name, patient.recordNumber, patient.document, patient.phone].join(' ')).includes(query));
    rows.innerHTML = matches.length ? matches.map(patient => `<a class="patient-row" href="#paciente/${patient.id}">
      <span class="patient-avatar">${icon('person')}</span>
      <span class="row-main"><strong>${escapeHtml(patient.name)}</strong><small>${escapeHtml(patient.recordNumber)}</small></span>
      <span class="patient-phone">${escapeHtml(patient.phone || 'Sin teléfono')}</span>${icon('next')}
    </a>`).join('') : emptyState(query ? 'Sin resultados' : 'Todavía no hay pacientes', query ?
      'Prueba con otro nombre o número de historia clínica.' : 'Registra un paciente para abrir su ficha clínica.');
  };
  input.addEventListener('input', draw);
  draw();
}

export function agendaHeading(date: string): string {
  return date === localDate() ? 'Hoy · ' + dateLabel(date) : dateLabel(date, true);
}
