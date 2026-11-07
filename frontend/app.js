const root = document.getElementById('app');
const modalRoot = document.getElementById('modal-root');
const toastEl = document.getElementById('toast');
const state = { user: null, projects: [], project: null, members: [], tasks: [], taskStatus: 'all', taskSearch: '' };
const labels = { owner: 'Владелец', manager: 'Управляющий', member: 'Участник', admin: 'Администратор', user: 'Пользователь', todo: 'К выполнению', in_progress: 'В работе', in_review: 'На ревью', done: 'Готово', low: 'Низкий', medium: 'Средний', high: 'Высокий', critical: 'Критический' };
const esc = (v = '') => String(v ?? '').replace(/[&<>"']/g, c => ({ '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;' })[c]);
const fmt = d => d ? new Date(d).toLocaleDateString('ru-RU', { day: '2-digit', month: 'short', year: 'numeric' }) : 'Не указан';
const inputDate = d => d ? new Date(new Date(d).getTime() - new Date(d).getTimezoneOffset() * 60000).toISOString().slice(0, 16) : '';
const safeUrl = v => { try { const url = new URL(v); return ['https:', 'http:'].includes(url.protocol) ? url.href : ''; } catch { return ''; } };
function notify(text, error = false) { toastEl.textContent = text; toastEl.className = 'show' + (error ? ' error' : ''); clearTimeout(notify.t); notify.t = setTimeout(() => toastEl.className = '', 4500); }
async function api(path, options = {}) {
  const { body, ...rest } = options;
  const res = await fetch('/api' + path, { credentials: 'same-origin', ...rest, headers: { 'Content-Type': 'application/json', ...(rest.headers || {}) }, ...(body !== undefined ? { body: JSON.stringify(body) } : {}) });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) { if (res.status === 401 && !path.startsWith('/auth/')) { state.user = null; navigate('/login'); } throw Error(data.error || `Ошибка ${res.status}`); }
  return data;
}
function navigate(path) { history.pushState({}, '', path); closeModal(); renderRoute(); }
window.addEventListener('popstate', renderRoute);
function isManager() { return state.user?.role === 'admin' || ['owner', 'manager'].includes(state.project?.my_role); }
function isOwner() { return state.project?.owner_id === state.user?.id; }
function header(active) {
  const logged = !!state.user;
  return `<div class="layout"><aside class="sidebar"><div class="side-label">МЕНЮ</div><nav class="menu">
    <a data-route="/" href="/" class="${active === '/' ? 'active' : ''}">⌂ <span>Обзор</span></a>
    <a data-route="/projects" href="/projects" class="${active.startsWith('/projects') ? 'active' : ''}">▦ <span>Проекты</span></a>
    ${state.user?.role === 'admin' ? `<a data-route="/admin" href="/admin" class="${active === '/admin' ? 'active' : ''}">⚙ <span>Админ-панель</span></a>` : ''}
    <a data-route="/about" href="/about" class="${active === '/about' ? 'active' : ''}">ⓘ <span>О системе</span></a>
    <a href="/swagger" target="_blank" rel="noopener">⌘ <span>Swagger API ↗</span></a>
  </nav><div class="side-bottom">${logged ? `<div class="account"><span class="avatar">${esc(state.user.name?.slice(0, 1).toUpperCase())}</span><span><b>${esc(state.user.name)}</b><small>${esc(state.user.email)}</small></span></div><button class="text-btn" data-action="logout">Выйти →</button>` : `<a data-route="/login" href="/login" class="btn secondary">Войти</a>`}</div></aside>
  <div class="main-column"><header class="topbar"><div><b>${active === '/' ? 'Обзор' : active.startsWith('/projects/') ? 'Проект' : active === '/projects' ? 'Проекты' : active === '/admin' ? 'Администрирование' : 'Информация'}</b></div><div class="top-actions">${logged ? `<span class="avatar mini">${esc(state.user.name?.slice(0, 1).toUpperCase())}</span>` : ''}</div></header><main id="page" class="page"></main></div></div>`;
}
function shell(active, content) { root.innerHTML = header(active); document.getElementById('page').innerHTML = content; }
function heading(kicker, title, text, actions = '') { return `<div class="page-heading"><div><div class="kicker">${esc(kicker)}</div><h1>${esc(title)}</h1><p>${esc(text)}</p></div><div class="heading-actions">${actions}</div></div>`; }
const errorNotice = err => notify(err.message || String(err), true);
async function renderRoute() {
  const path = location.pathname;
  try {
    if (!state.user && !['/login', '/register', '/about'].includes(path)) return navigate('/login');
    if (path === '/login' || path === '/register') return renderAuth(path);
    if (path === '/') return renderDashboard();
    if (path === '/projects') return renderProjects();
    const match = path.match(/^\/projects\/(\d+)$/);
    if (match) return renderProject(Number(match[1]));
    if (path === '/admin') return renderAdmin();
    if (path === '/about') return renderAbout();
    shell(path, heading('404', 'Страница не найдена', 'Проверьте адрес.') + `<button class="btn" data-route="/">На главную</button>`);
  } catch (err) { errorNotice(err); }
}
function renderAuth(path) {
  const reg = path === '/register';
  root.innerHTML = `<div class="auth-layout"><main class="auth-main"><div class="auth-form"><h2>${reg ? 'Регистрация' : 'Вход'}</h2><p>${reg ? 'Создайте аккаунт, чтобы управлять совместными проектами.' : 'Войдите в ваше рабочее пространство.'}</p>
  <form id="auth-form">${reg ? `<label>Ваше имя<input name="name" required minlength="2" maxlength="100" placeholder="Иван Иванов" autocomplete="name"></label>` : ''}<label>Email<input name="email" type="email" required placeholder="you@example.com" autocomplete="email"></label><label>Пароль<input name="password" type="password" required minlength="${reg ? '10' : '1'}" placeholder="${reg ? 'Не менее 10 символов' : 'Ваш пароль'}" autocomplete="${reg ? 'new-password' : 'current-password'}"></label><button class="btn full" type="submit">${reg ? 'Зарегистрироваться' : 'Войти'} →</button></form><p class="switch-auth">${reg ? 'Уже есть аккаунт?' : 'Впервые здесь?'} <a href="${reg ? '/login' : '/register'}" data-route="${reg ? '/login' : '/register'}">${reg ? 'Войти' : 'Создать аккаунт'}</a></p></div></main></div>`;
}
async function renderDashboard() {
  shell('/', heading('ГЛАВНАЯ', 'Обзор задач', 'Проекты и текущие задачи.', `<button class="btn" data-action="new-project">+ Новый проект</button>`) + `<div class="loading">Загружаем статистику…</div>`);
  const [s, p] = await Promise.all([api('/dashboard'), api('/projects')]); state.projects = p.projects;
  const cards = [['projects', 'Проектов', '▦'], ['tasks', 'Всего задач', '◈'], ['assigned_to_me', 'Мои активные', '◎'], ['in_review', 'На ревью', '✓'], ['done', 'Завершено', '★']];
  const stats = cards.map(([key, title, icon]) => `<div class="stat-card"><span class="stat-icon">${icon}</span><div class="stat-number">${s[key]}</div><div class="muted">${title}</div></div>`).join('');
  document.getElementById('page').innerHTML = heading('РАБОЧИЙ СТОЛ', 'Хорошего рабочего дня 👋', 'Общий обзор проектов и задач вашей команды.', `<button class="btn" data-action="new-project">+ Новый проект</button>`) + `<div class="stats-grid">${stats}</div><section class="section-heading"><div><h2>Ваши проекты</h2><p>Быстрый доступ к рабочим пространствам</p></div><a href="/projects" data-route="/projects">Все проекты →</a></section>${projectsGrid(p.projects.slice(0, 6))}<div class="tip"><b>Как это работает?</b><span>Управляющий создаёт задачу → исполнитель прикладывает результат → другой управляющий проверяет и одобряет работу.</span><a href="/about" data-route="/about">Подробнее →</a></div>`;
}
function projectsGrid(projects) {
  if (!projects.length) return `<div class="empty"><span>▦</span><h3>Пока нет проектов</h3><p>Создайте первый проект, чтобы пригласить команду и начать работу.</p><button class="btn" data-action="new-project">+ Создать проект</button></div>`;
  return `<div class="projects-grid">${projects.map(p => `<article class="project-card" data-route="/projects/${p.id}" tabindex="0" role="link"><div class="project-top"><span class="project-logo">${esc(p.name[0]?.toUpperCase())}</span><span class="role-tag">${esc(labels[p.my_role] || p.my_role)}</span></div><h3>${esc(p.name)}</h3><p>${esc(p.description || 'Описание пока не добавлено')}</p><div class="project-bottom"><span>♧ ${p.members_count} участников</span><span>◈ ${p.tasks_count} задач</span><span class="arrow">↗</span></div></article>`).join('')}</div>`;
}
async function renderProjects() {
  shell('/projects', heading('РАБОЧИЕ ПРОСТРАНСТВА', 'Проекты', 'Все доступные вам проекты.', `<button class="btn" data-action="new-project">+ Создать проект</button>`) + `<div class="loading">Загрузка…</div>`);
  const r = await api('/projects'); state.projects = r.projects;
  document.getElementById('page').innerHTML = heading('РАБОЧИЕ ПРОСТРАНСТВА', 'Проекты', 'Все доступные вам проекты.', `<button class="btn" data-action="new-project">+ Создать проект</button>`) + projectsGrid(r.projects);
}
function taskCard(t) {
  const reviewed = t.latest_review;
  const manager = isManager();
  const mine = t.assignee_id === state.user.id;
  const tags = (t.tags || '').split(',').map(x => x.trim()).filter(Boolean).slice(0, 4);
  const buttons = [];
  if (t.status === 'todo' && mine) buttons.push(`<button class="btn xs secondary" data-action="start-task" data-id="${t.id}">Начать</button>`);
  if ((t.status === 'todo' || t.status === 'in_progress') && mine) buttons.push(`<button class="btn xs" data-action="submit-task" data-id="${t.id}">На ревью →</button>`);
  if (manager && ['todo', 'in_progress'].includes(t.status)) buttons.push(`<button class="btn xs secondary" data-action="edit-task" data-id="${t.id}">Изменить</button>`);
  if (manager && t.status === 'in_review' && reviewed?.submitted_by !== state.user.id) buttons.push(`<button class="btn xs" data-action="review-task" data-id="${t.id}">Проверить</button>`);
  return `<article class="task-card"><div class="task-top"><span class="ticket">TT-${t.id}</span><span class="priority ${esc(t.urgency)}" title="Срочность по дедлайну / вручную"></span></div><h4>${esc(t.title)}</h4><p class="description">${esc(t.description || 'Нет описания')}</p><div class="tag-row">${t.epic ? `<span class="tag purple">${esc(t.epic)}</span>` : ''}${tags.map(x => `<span class="tag">${esc(x)}</span>`).join('')}</div><div class="task-meta"><span>◷ ${fmt(t.due_at)}</span><span class="priority-text ${esc(t.urgency)}" title="Назначенный приоритет: ${esc(labels[t.priority] || t.priority)}">${esc(labels[t.effective_priority || t.priority] || t.effective_priority || t.priority)}</span></div><div class="task-meta"><span>◉ ${esc(t.assignee_name || 'Не назначен')}</span><span>${fmt(t.created_at)}</span></div>${reviewed && ['in_review', 'in_progress', 'done'].includes(t.status) ? `<div class="review-note"><b>${reviewed.status === 'pending' ? 'Ожидает проверки' : reviewed.status === 'approved' ? 'Одобрено' : 'Отклонено'}</b>${reviewed.description ? `<p>${esc(reviewed.description)}</p>` : ''}${reviewed.evidence_url && safeUrl(reviewed.evidence_url) ? `<a target="_blank" rel="noopener noreferrer" href="${esc(safeUrl(reviewed.evidence_url))}">Материалы ↗</a>` : ''}${reviewed.comment ? `<p><b>Комментарий:</b> ${esc(reviewed.comment)}</p>` : ''}</div>` : ''}<div class="task-actions">${buttons.join('')}</div></article>`;
}
function taskBoard() {
  const columns = [['todo', 'К выполнению', 'slate'], ['in_progress', 'В работе', 'blue'], ['in_review', 'На ревью', 'yellow'], ['done', 'Готово', 'green']];
  const filtered = state.tasks.filter(t => (state.taskStatus === 'all' || t.status === state.taskStatus) && `${t.title} ${t.description} ${t.tags}`.toLowerCase().includes(state.taskSearch.toLowerCase()));
  return `<div class="board">${columns.map(([key, name, color]) => {const tasks = filtered.filter(t => t.status === key);return `<section class="board-col"><div class="col-head"><div><span class="col-dot ${color}"></span><b>${name}</b></div><span>${tasks.length}</span></div><div class="col-body">${tasks.length ? tasks.map(taskCard).join('') : `<div class="col-empty">Нет задач</div>`}</div></section>`;}).join('')}</div>`;
}
async function renderProject(id) {
  shell('/projects/' + id, `<div class="loading">Открываем проект…</div>`);
  const [p, t] = await Promise.all([api(`/projects/${id}`), api(`/projects/${id}/tasks`)]);
  state.project = p.project; state.members = p.members; state.tasks = t.tasks;
  paintProject();
}
function paintProject() {
  const p = state.project, manage = isManager(), owner = isOwner();
  const action = `<button class="btn secondary" data-action="invite">+ Пригласить</button>${manage ? `<button class="btn" data-action="new-task">+ Новая задача</button>` : ''}`;
  const memberCards = state.members.map(m => `<div class="member"><span class="avatar">${esc(m.name[0]?.toUpperCase())}</span><div class="member-info"><b>${esc(m.name)}</b><small>${esc(m.email)}</small></div><span class="role-tag">${esc(labels[m.role] || m.role)}</span>${owner && m.role !== 'owner' ? `<button class="icon-btn" title="Изменить роль" data-action="member-role" data-id="${m.user_id}">⚙</button><button class="icon-btn danger" title="Удалить из проекта" data-action="member-remove" data-id="${m.user_id}">×</button>` : ''}${!owner && m.user_id === state.user.id && m.role !== 'owner' ? `<button class="icon-btn" title="Покинуть проект" data-action="member-remove" data-id="${m.user_id}">Выйти</button>` : ''}</div>`).join('');
  document.getElementById('page').innerHTML = `<div class="back"><a href="/projects" data-route="/projects">← Все проекты</a></div>${heading('ПРОЕКТ / ' + (labels[p.my_role] || p.my_role), p.name, p.description || 'Общее пространство команды', action)}<div class="project-tools"><div><span class="tiny-stat">♧ ${p.members_count} участников</span><span class="tiny-stat">◈ ${p.tasks_count} задач</span><span class="tiny-stat">Создан ${fmt(p.created_at)}</span></div><div>${manage ? `<button class="text-btn" data-action="edit-project">✎ Изменить</button>` : ''}${owner || p.created_by === state.user.id ? `<button class="text-btn danger-text" data-action="delete-project">Удалить проект</button>` : ''}</div></div>
  <div class="section-heading compact"><div><h2>Доска задач</h2><p>Сроки, приоритеты и проверки</p></div><div class="filters"><input id="task-search" placeholder="Поиск задач…" value="${esc(state.taskSearch)}" aria-label="Поиск задач"><select id="task-status" aria-label="Фильтр статуса">${[['all', 'Все статусы'], ['todo', 'К выполнению'], ['in_progress', 'В работе'], ['in_review', 'На ревью'], ['done', 'Готово']].map(([id, name]) => `<option value="${id}" ${state.taskStatus === id ? 'selected' : ''}>${name}</option>`).join('')}</select></div></div>${taskBoard()}
  <div class="members-section"><div class="section-heading"><div><h2>Команда проекта</h2><p>Владелец, управляющие и участники</p></div>${manage ? `<button class="btn secondary" data-action="invite">+ Пригласить</button>` : ''}</div><div class="member-list">${memberCards}</div></div>`;
}
async function refreshProject() { if (!state.project) return; const [p, t] = await Promise.all([api(`/projects/${state.project.id}`), api(`/projects/${state.project.id}/tasks`)]);state.project = p.project;state.members = p.members;state.tasks = t.tasks;paintProject(); }
async function renderAdmin() {
  if (state.user.role !== 'admin') return navigate('/');
  shell('/admin', heading('УПРАВЛЕНИЕ', 'Пользователи', 'Только глобальные администраторы могут менять роли пользователей.') + `<div class="loading">Загрузка…</div>`);
  const { users } = await api('/admin/users');
  document.getElementById('page').innerHTML = heading('УПРАВЛЕНИЕ', 'Пользователи', 'Только глобальные администраторы могут менять роли пользователей.') + `<div class="table-wrap"><table><thead><tr><th>Пользователь</th><th>Email</th><th>Роль</th><th>Зарегистрирован</th><th></th></tr></thead><tbody>${users.map(u => `<tr><td><span class="avatar mini">${esc(u.name[0]?.toUpperCase())}</span> ${esc(u.name)}</td><td>${esc(u.email)}</td><td><span class="role-tag">${esc(labels[u.role])}</span></td><td>${fmt(u.created_at)}</td><td>${u.id === state.user.id ? 'Это вы' : `<button class="btn xs secondary" data-action="global-role" data-id="${u.id}" data-role="${esc(u.role)}">Сменить роль</button>`}</td></tr>`).join('')}</tbody></table></div>`;
}
function renderAbout() {
  shell('/about', heading('СПРАВКА', 'О приложении', 'Управление задачами небольших команд.') + `<div class="info-grid"><section class="info-card"><h3>Проекты и доступ</h3><p>Владелец управляет проектом, назначает управляющих и может передать права владения. Управляющие создают задачи и проверяют результат. Участники выполняют назначенную работу.</p></section><section class="info-card"><h3>Проверка задач</h3><p>Исполнитель не может завершить задачу напрямую: он отправляет отчёт и/или ссылку на результат. Другой управляющий принимает работу или возвращает её в работу.</p></section><section class="info-card"><h3>Автоматическая срочность</h3><p>Более 7 дней — зелёный, неделя — жёлтый, 3 дня — красный, 24 часа — мигающий красный. При приближении срока отображаемый приоритет повышается автоматически; исходный приоритет сохраняется. Управляющий может изменить индикатор вручную.</p></section><section class="info-card"><h3>Технологии</h3><p>Go/Gin, GORM, PostgreSQL, JWT с серверными сессиями, JavaScript, Docker Compose и OpenAPI/Swagger.</p><a href="/swagger" target="_blank" rel="noopener">Открыть Swagger ↗</a></section></div>`);
}
function modal(title, content, submit = 'Сохранить') {
  modalRoot.innerHTML = `<div class="modal-backdrop"><section class="modal" role="dialog" aria-modal="true" aria-label="${esc(title)}"><div class="modal-head"><div><h2>${esc(title)}</h2></div><button class="icon-btn" data-action="close-modal" aria-label="Закрыть">×</button></div><form id="modal-form">${content}<div class="modal-actions"><button class="btn secondary" type="button" data-action="close-modal">Отмена</button><button class="btn" type="submit">${esc(submit)}</button></div></form></section></div>`;
  const back = modalRoot.querySelector('.modal-backdrop');back.addEventListener('click', e => {if (e.target === back) closeModal();});
  modalRoot.querySelector('input,textarea,select')?.focus();
}
function closeModal() { modalRoot.innerHTML = ''; }
function field(name, label, value = '', extra = '') { return `<label>${esc(label)}<input name="${esc(name)}" value="${esc(value)}" ${extra}></label>`; }
function textarea(name, label, value = '') { return `<label>${esc(label)}<textarea name="${esc(name)}" rows="3">${esc(value)}</textarea></label>`; }
function select(name, label, items, value) {return `<label>${esc(label)}<select name="${esc(name)}">${items.map(([v, text]) => `<option value="${esc(v)}" ${v === value ? 'selected' : ''}>${esc(text)}</option>`).join('')}</select></label>`;}
function projectModal(edit = false) {const p = edit ? state.project : {}; modal(edit ? 'Редактировать проект' : 'Новый проект', field('name', 'Название', p.name, 'required minlength="2" maxlength="120" placeholder="Сайт компании"') + textarea('description', 'Описание', p.description)); modalRoot.dataset.form = edit ? 'edit-project' : 'create-project';}
function inviteModal() {modal('Пригласить участника', `<p class="form-help">Пользователь должен предварительно зарегистрироваться. Новый участник получает роль «Участник».</p>` + field('email', 'Email пользователя', '', 'type="email" required placeholder="colleague@example.com"'), 'Пригласить');modalRoot.dataset.form = 'invite';}
function taskModal(id = null) {
  const t = id ? state.tasks.find(x => x.id === id) : null;
  modal(t ? 'Изменить задачу' : 'Новая задача', field('title', 'Название задачи', t?.title, 'required minlength="2" maxlength="160"') + textarea('description', 'Описание задачи', t?.description) + `<div class="form-row">${select('priority', 'Базовый приоритет', [['low', 'Низкий'], ['medium', 'Средний'], ['high', 'Высокий'], ['critical', 'Критический']], t?.priority || 'medium')}${select('urgency_override', 'Индикатор срока', [['auto', 'Автоматически'], ['green', 'Зелёный'], ['yellow', 'Жёлтый'], ['red', 'Красный'], ['critical', 'Мигающий красный']], t?.urgency_override || 'auto')}</div>` + `<div class="form-row">${field('epic', 'Эпик / тип', t?.epic, 'maxlength="100" placeholder="dev, design…"')}${field('tags', 'Теги через запятую', t?.tags, 'maxlength="400" placeholder="backend, bug"')}</div>` + `<div class="form-row">${select('assignee_id', 'Исполнитель', [['', 'Не назначен'], ...state.members.map(m => [String(m.user_id), m.name])], t?.assignee_id ? String(t.assignee_id) : '')}${field('due_at', 'Дедлайн', inputDate(t?.due_at), 'type="datetime-local"')}</div>`, t ? 'Сохранить задачу' : 'Создать задачу');
  modalRoot.dataset.form = t ? 'edit-task' : 'create-task';modalRoot.dataset.task = String(id || '');
}
function submitModal(id) { modal('Отправить на ревью', `<p class="form-help">Опишите выполненную работу и/или укажите ссылку на GitHub, Figma или другой результат. Другой управляющий должен подтвердить выполнение.</p>` + textarea('description', 'Что сделано') + field('evidence_url', 'Ссылка на результат (необязательно)', '', 'type="url" placeholder="https://github.com/…"'), 'Отправить на проверку'); modalRoot.dataset.form = 'submit-task';modalRoot.dataset.task = String(id);}
function reviewModal(id) {const t = state.tasks.find(x => x.id === id);modal('Проверить задачу', `<div class="review-detail"><b>${esc(t?.title)}</b><p>${esc(t?.latest_review?.description || 'Описание не приложено')}</p>${safeUrl(t?.latest_review?.evidence_url) ? `<a target="_blank" rel="noopener noreferrer" href="${esc(safeUrl(t.latest_review.evidence_url))}">Открыть материалы ↗</a>` : ''}</div>` + select('decision', 'Решение', [['approved', 'Одобрить выполнение'], ['rejected', 'Вернуть в работу']], 'approved') + textarea('comment', 'Комментарий проверяющего'), 'Сохранить решение');modalRoot.dataset.form = 'review-task';modalRoot.dataset.task = String(id);}
function memberRoleModal(id) {const m = state.members.find(x => x.user_id === id);modal(`Роль: ${m?.name || 'участник'}`, `<p class="form-help">Назначение владельцем передаст этому участнику право удалять проект; вы станете управляющим.</p>` + select('role', 'Роль в проекте', [['member', 'Участник'], ['manager', 'Управляющий'], ['owner', 'Передать владение']], m?.role || 'member'), 'Изменить роль');modalRoot.dataset.form = 'member-role';modalRoot.dataset.user = String(id);}
function formValues(form) {return Object.fromEntries(new FormData(form).entries());}
function taskPayload(f) {return {title:f.title,description:f.description,priority:f.priority,urgency_override:f.urgency_override,epic:f.epic,tags:f.tags,assignee_id:f.assignee_id ? Number(f.assignee_id) : null,due_at:f.due_at ? new Date(f.due_at).toISOString() : null};}
document.addEventListener('submit', async event => {
  const form = event.target;if (!(form instanceof HTMLFormElement)) return;
  event.preventDefault();const f = formValues(form);const kind = form.id === 'auth-form' ? location.pathname.slice(1) : modalRoot.dataset.form;
  const btn = form.querySelector('button[type=submit]');btn.disabled = true;
  try {
    let message = 'Изменения сохранены'; const id = state.project?.id;
    switch (kind) {
      case 'register':case 'login': {const {user} = await api('/auth/' + kind, {method:'POST',body:f});state.user = user;notify(kind === 'register' ? 'Добро пожаловать!' : 'Вход выполнен');return navigate('/');}
      case 'create-project': {const {project} = await api('/projects', {method:'POST',body:f});message='Проект создан';closeModal();notify(message);return navigate(`/projects/${project.id}`);}
      case 'edit-project': await api(`/projects/${id}`, {method:'PATCH',body:f});break;
      case 'invite': await api(`/projects/${id}/invitations`, {method:'POST',body:f});message='Участник приглашён';break;
      case 'create-task': await api(`/projects/${id}/tasks`, {method:'POST',body:taskPayload(f)});message='Задача создана';break;
      case 'edit-task': await api(`/projects/${id}/tasks/${modalRoot.dataset.task}`, {method:'PATCH',body:taskPayload(f)});break;
      case 'submit-task': await api(`/projects/${id}/tasks/${modalRoot.dataset.task}/submit`, {method:'POST',body:f});message='Отправлено на ревью';break;
      case 'review-task': await api(`/projects/${id}/tasks/${modalRoot.dataset.task}/review`, {method:'POST',body:f});message=f.decision === 'approved' ? 'Задача завершена' : 'Задача возвращена в работу';break;
      case 'member-role': await api(`/projects/${id}/members/${modalRoot.dataset.user}`, {method:'PATCH',body:f});message='Роль изменена';break;
      default: throw Error('Неизвестная форма');
    }
    closeModal();notify(message);await refreshProject();
  } catch(err) {errorNotice(err);} finally { btn.disabled = false; }
});
document.addEventListener('click', async event => {
  const target = event.target.closest('[data-action], [data-route]');if (!target) return;
  if (target.dataset.route) {event.preventDefault();return navigate(target.dataset.route);}
  const action = target.dataset.action; const id = Number(target.dataset.id);
  if (action === 'close-modal') return closeModal();
  try {
    switch (action) {
      case 'logout': await api('/auth/logout', {method:'POST'});state.user=null;notify('Вы вышли из аккаунта');navigate('/login');break;
      case 'new-project':projectModal();break;
      case 'edit-project':projectModal(true);break;
      case 'invite':inviteModal();break;
      case 'new-task':taskModal();break;
      case 'edit-task':taskModal(id);break;
      case 'submit-task':submitModal(id);break;
      case 'review-task':reviewModal(id);break;
      case 'member-role':memberRoleModal(id);break;
      case 'start-task':await api(`/projects/${state.project.id}/tasks/${id}`,{method:'PATCH',body:{status:'in_progress'}});notify('Задача взята в работу');await refreshProject();break;
      case 'member-remove':if (confirm('Удалить участника из проекта?')){await api(`/projects/${state.project.id}/members/${id}`,{method:'DELETE'});if (id===state.user.id) return navigate('/projects');await refreshProject();notify('Участник удалён');}break;
      case 'delete-project':if (confirm(`Безвозвратно удалить проект «${state.project.name}» и все задачи?`)){await api(`/projects/${state.project.id}`,{method:'DELETE'});notify('Проект удалён');navigate('/projects');}break;
      case 'global-role':if (confirm('Изменить глобальную роль пользователя?')){await api(`/admin/users/${id}/role`,{method:'PATCH',body:{role:target.dataset.role === 'admin' ? 'user':'admin'}});notify('Роль обновлена');await renderAdmin();}break;
    }
  } catch(err) {errorNotice(err);}
});
document.addEventListener('change', event => {if (event.target.id === 'task-status'){state.taskStatus = event.target.value;document.querySelector('.board')?.replaceWith(makeBoard());}});
function makeBoard() {const box = document.createElement('div');box.innerHTML = taskBoard();return box.firstElementChild;}
document.addEventListener('input', event => {if (event.target.id === 'task-search'){state.taskSearch=event.target.value;document.querySelector('.board')?.replaceWith(makeBoard());}});
document.addEventListener('keydown', event => {if (event.key === 'Escape')closeModal();if (event.key === 'Enter' && event.target.matches('[role=link][data-route]'))navigate(event.target.dataset.route);});
(async()=>{try{const r=await api('/auth/me');state.user=r.user;}catch{}renderRoute();})();