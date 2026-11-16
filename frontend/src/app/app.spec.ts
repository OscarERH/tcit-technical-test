import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { ComponentFixture, TestBed } from '@angular/core/testing';
import { App } from './app';
import { provideIcons } from './icons';
import { AppConfig, Post } from './posts/post.model';

const post = (id: string, name: string, description: string, status = 'draft'): Post => ({
  id,
  name,
  description,
  status,
  createdAt: '2026-01-01T00:00:00Z',
  attachments: [],
});

const POSTS = [post('1', 'POST 1', 'Hola como están'), post('2', 'Campaña', 'Invierno', 'published')];

const NO_INTEGRATIONS: AppConfig = {
  features: { attachments: false, briefExtraction: false, workflow: false },
  transitions: [],
};

const WITH_INTEGRATIONS: AppConfig = {
  features: { attachments: false, briefExtraction: true, workflow: true },
  transitions: [
    { name: 'publish', from: ['draft'], to: 'published' },
    { name: 'archive', from: ['published'], to: 'archived' },
  ],
};

/** Whole-view tests: real components and store, with only HTTP replaced. */
describe('Posts view', () => {
  let fixture: ComponentFixture<App>;
  let http: HttpTestingController;
  let view: HTMLElement;

  beforeEach(() => {
    TestBed.configureTestingModule({
      providers: [provideHttpClient(), provideHttpClientTesting(), provideIcons()],
    });
    http = TestBed.inject(HttpTestingController);
  });

  afterEach(() => http.verify());

  async function render(posts: Post[] = POSTS, config: AppConfig = NO_INTEGRATIONS): Promise<void> {
    fixture = TestBed.createComponent(App);
    view = fixture.nativeElement;
    fixture.detectChanges();
    http.expectOne('/api/config').flush(config);
    http.expectOne('/api/posts').flush(posts);
    await settle();
  }

  /**
   * Waits until the view reflects the latest responses. The store updates its
   * signals a few microtasks after a response is flushed, so the pending
   * promise chain must run before asking Angular whether it is stable.
   */
  async function settle(): Promise<void> {
    await new Promise((resolve) => setTimeout(resolve));
    await fixture.whenStable();
  }

  const text = (element: Element | null) => element?.textContent?.replace(/\s+/g, ' ').trim() ?? '';
  const rows = () => [...view.querySelectorAll('tr[mat-row]')];
  const rowNames = () => rows().map((row) => text(row.querySelector('.name')));
  const headers = () => [...view.querySelectorAll('th')].map(text);
  const button = (label: string, root: Element = view) =>
    [...root.querySelectorAll('button')].find((b) => text(b).includes(label)) as HTMLButtonElement;
  const filterInput = () => view.querySelector('app-post-filter input') as HTMLInputElement;
  const nameInput = () => view.querySelector('input[formcontrolname=name]') as HTMLInputElement;
  const descriptionInput = () =>
    view.querySelector('textarea[formcontrolname=description]') as HTMLTextAreaElement;

  async function type(input: HTMLInputElement | HTMLTextAreaElement, value: string): Promise<void> {
    input.value = value;
    input.dispatchEvent(new Event('input'));
    await settle();
  }

  async function click(element: HTMLElement): Promise<void> {
    element.click();
    await settle();
  }

  it('shows the three parts of the view: filter, list and form', async () => {
    await render();

    expect(filterInput()).toBeTruthy();
    expect(button('Buscar')).toBeTruthy();
    expect(headers()).toEqual(['Nombre', 'Descripción', 'Acción']);
    expect(nameInput()).toBeTruthy();
    expect(descriptionInput()).toBeTruthy();
    expect(button('Crear')).toBeTruthy();
  });

  it('lists every post with its name and description', async () => {
    await render();

    expect(rows().map((row) => text(row.querySelector('.mat-column-description')))).toEqual([
      'Hola como están',
      'Invierno',
    ]);
    expect(rowNames()).toEqual(['POST 1', 'Campaña']);
    expect(text(view.querySelector('.list-pane .pane-header p'))).toBe('2 posts');
  });

  it('filters by name when "Buscar" is pressed, without calling the API', async () => {
    await render();

    await type(filterInput(), 'camp');
    expect(rowNames()).toEqual(['POST 1', 'Campaña']); // not applied until submitted

    await click(button('Buscar'));
    expect(rowNames()).toEqual(['Campaña']);
    expect(text(view.querySelector('.list-pane .pane-header p'))).toBe('1 de 2 posts');
    http.expectNone(() => true);
  });

  it('explains an empty result and lets the user clear the filter', async () => {
    await render();

    await type(filterInput(), 'Hola');
    await click(button('Buscar'));

    expect(rows()).toEqual([]);
    expect(text(view.querySelector('.empty p'))).toBe(
      'Ningún post tiene "Hola" en su nombre. El filtro no busca en la descripción.',
    );

    await click(button('Limpiar filtro'));
    expect(rowNames()).toEqual(['POST 1', 'Campaña']);
    expect(filterInput().value).toBe('');
  });

  it('removes the filter as soon as the input is emptied', async () => {
    await render();
    await type(filterInput(), 'camp');
    await click(button('Buscar'));

    await type(filterInput(), '');

    expect(rowNames()).toEqual(['POST 1', 'Campaña']);
  });

  it('invites to create the first post when there are none', async () => {
    await render([]);

    expect(text(view.querySelector('.empty p'))).toBe('Aún no hay posts. Crea el primero con el formulario.');
    expect(button('Limpiar filtro')).toBeUndefined();
  });

  it('creates a post from the form, adds it to the list and resets the form', async () => {
    await render();
    expect(button('Crear').disabled).toBe(true);

    await type(nameInput(), 'Nuevo');
    expect(button('Crear').disabled).toBe(true); // description still missing
    await type(descriptionInput(), 'Desde el formulario');
    expect(button('Crear').disabled).toBe(false);

    button('Crear').click();
    const request = http.expectOne({ method: 'POST', url: '/api/posts' });
    expect(request.request.body).toEqual({ name: 'Nuevo', description: 'Desde el formulario' });
    request.flush(post('3', 'Nuevo', 'Desde el formulario'));
    await settle();

    expect(rowNames()).toEqual(['POST 1', 'Campaña', 'Nuevo']);
    expect(nameInput().value).toBe('');
    expect(descriptionInput().value).toBe('');
  });

  it('does not allow creating a post made only of spaces', async () => {
    await render();

    await type(nameInput(), '   ');
    await type(descriptionInput(), '   ');

    expect(button('Crear').disabled).toBe(true);
  });

  it('keeps the typed values when creating fails', async () => {
    await render();
    await type(nameInput(), 'Nuevo');
    await type(descriptionInput(), 'x');

    button('Crear').click();
    http
      .expectOne('/api/posts')
      .flush({ error: { message: 'Invalid post' } }, { status: 400, statusText: 'Bad Request' });
    await settle();

    expect(nameInput().value).toBe('Nuevo');
    expect(rowNames()).toEqual(['POST 1', 'Campaña']);
  });

  it('deletes a post from its row', async () => {
    await render();

    (view.querySelector('button[aria-label="Eliminar POST 1"]') as HTMLButtonElement).click();
    http.expectOne({ method: 'DELETE', url: '/api/posts/1' }).flush(POSTS[0]);
    await settle();

    expect(rowNames()).toEqual(['Campaña']);
  });

  it('hides the optional integrations when they are not available', async () => {
    await render();

    expect(headers()).not.toContain('Estado');
    expect(button('Publicar')).toBeUndefined();
    expect(button('Completar con IA')).toBeUndefined();
    expect(view.querySelector('button[aria-label^="Adjuntar"]')).toBeNull();
  });

  it('shows status and lifecycle actions when the workflow is available', async () => {
    await render(POSTS, WITH_INTEGRATIONS);

    expect(headers()).toEqual(['Nombre', 'Descripción', 'Estado', 'Acción']);
    expect(rows().map((row) => text(row.querySelector('.mat-column-status')))).toEqual([
      'Borrador',
      'Publicado',
    ]);
    expect(button('Publicar', rows()[0])).toBeTruthy();
    expect(button('Archivar', rows()[1])).toBeTruthy();
    expect(button('Completar con IA')).toBeTruthy();

    button('Publicar', rows()[0]).click();
    http
      .expectOne({ method: 'POST', url: '/api/posts/1/transitions' })
      .flush({ ...POSTS[0], status: 'published' });
    await settle();

    expect(text(rows()[0].querySelector('.mat-column-status'))).toBe('Publicado');
    expect(button('Archivar', rows()[0])).toBeTruthy();
  });
});
