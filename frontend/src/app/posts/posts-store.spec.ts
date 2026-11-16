import { provideHttpClient } from '@angular/common/http';
import { HttpTestingController, provideHttpClientTesting } from '@angular/common/http/testing';
import { TestBed } from '@angular/core/testing';
import { AppConfig, Post } from './post.model';
import { PostsStore } from './posts-store';

const post = (id: string, name: string, extra: Partial<Post> = {}): Post => ({
  id,
  name,
  description: `Descripción de ${name}`,
  status: 'draft',
  createdAt: '2026-01-01T00:00:00Z',
  attachments: [],
  ...extra,
});

const POSTS = [post('1', 'Viaje a Chiloé'), post('2', 'Receta'), post('3', 'Hola mundo')];

const CONFIG: AppConfig = {
  features: { attachments: true, briefExtraction: true, workflow: true },
  transitions: [
    { name: 'publish', from: ['draft'], to: 'published' },
    { name: 'archive', from: ['published'], to: 'archived' },
  ],
};

describe('PostsStore', () => {
  let store: PostsStore;
  let http: HttpTestingController;

  beforeEach(() => {
    TestBed.configureTestingModule({ providers: [provideHttpClient(), provideHttpClientTesting()] });
    store = TestBed.inject(PostsStore);
    http = TestBed.inject(HttpTestingController);
  });

  // Fails the test if the store made a request the test did not expect.
  afterEach(() => http.verify());

  /** Starts the store and answers its two initial requests. */
  async function load(posts: Post[] = POSTS, config: AppConfig = CONFIG): Promise<void> {
    const loading = store.load();
    http.expectOne('/api/config').flush(config);
    http.expectOne('/api/posts').flush(posts);
    await loading;
  }

  const names = () => store.filteredPosts().map((p) => p.name);

  describe('loading', () => {
    it('requests the full list only once, however many times load() is called', async () => {
      const first = store.load();
      const second = store.load();
      expect(store.status()).toBe('loading');

      http.expectOne('/api/config').flush(CONFIG);
      // expectOne throws if the list had been requested more than once.
      http.expectOne('/api/posts').flush(POSTS);
      await Promise.all([first, second]);

      await store.load();
      http.expectNone('/api/posts');

      expect(store.status()).toBe('loaded');
      expect(store.total()).toBe(3);
    });

    it('reports a failed load', async () => {
      const loading = store.load();
      http.expectOne('/api/config').flush(CONFIG);
      http.expectOne('/api/posts').flush('boom', { status: 500, statusText: 'Server Error' });
      await loading;

      expect(store.status()).toBe('failed');
      expect(store.error()).toBe('No se pudo completar la operación. Intenta nuevamente.');
      expect(store.filteredPosts()).toEqual([]);
    });

    it('still lists posts when the optional integrations cannot be loaded', async () => {
      const loading = store.load();
      http.expectOne('/api/config').flush('boom', { status: 502, statusText: 'Bad Gateway' });
      http.expectOne('/api/posts').flush(POSTS);
      await loading;

      expect(store.status()).toBe('loaded');
      expect(store.error()).toBeNull();
      expect(store.features()).toEqual({ attachments: false, briefExtraction: false, workflow: false });
      expect(store.transitionsFor(POSTS[0])).toEqual([]);
    });
  });

  describe('filter', () => {
    beforeEach(() => load());

    it('filters by name locally, without calling the API', () => {
      store.setFilter('rece');

      expect(names()).toEqual(['Receta']);
      http.expectNone(() => true);
    });

    it('ignores case and surrounding spaces', () => {
      store.setFilter('  HOLA ');
      expect(names()).toEqual(['Hola mundo']);
    });

    it('matches the name only, never the description', () => {
      store.setFilter('descripción');
      expect(names()).toEqual([]);
    });

    it('shows every post again when the filter is cleared', () => {
      store.setFilter('rece');
      store.setFilter('');
      expect(names()).toEqual(['Viaje a Chiloé', 'Receta', 'Hola mundo']);
    });

    it('keeps applying to posts created afterwards', async () => {
      store.setFilter('rece');

      const creating = store.create({ name: 'Receta nueva', description: 'x' });
      http.expectOne('/api/posts').flush(post('4', 'Receta nueva'));
      await creating;

      expect(names()).toEqual(['Receta', 'Receta nueva']);
      expect(store.total()).toBe(4);
    });
  });

  describe('create', () => {
    beforeEach(() => load());

    it('appends the post returned by the API without reloading the list', async () => {
      const creating = store.create({ name: 'Nuevo', description: 'x' });

      const request = http.expectOne({ method: 'POST', url: '/api/posts' });
      expect(request.request.body).toEqual({ name: 'Nuevo', description: 'x' });
      request.flush(post('4', 'Nuevo'));

      expect(await creating).toBe(true);
      expect(names()).toEqual(['Viaje a Chiloé', 'Receta', 'Hola mundo', 'Nuevo']);
    });

    it('reports the API error and leaves the list untouched', async () => {
      const creating = store.create({ name: '', description: '' });
      http
        .expectOne('/api/posts')
        .flush({ error: { message: 'Invalid post' } }, { status: 400, statusText: 'Bad Request' });

      expect(await creating).toBe(false);
      expect(store.error()).toBe('Invalid post');
      expect(store.total()).toBe(3);
    });
  });

  describe('remove', () => {
    beforeEach(() => load());

    it('removes the post returned by the API', async () => {
      const removing = store.remove('2');
      expect(store.isPending('2')).toBe(true);

      http.expectOne({ method: 'DELETE', url: '/api/posts/2' }).flush(POSTS[1]);
      await removing;

      expect(names()).toEqual(['Viaje a Chiloé', 'Hola mundo']);
      expect(store.isPending('2')).toBe(false);
    });

    it('ignores a second click while the first request is in flight', async () => {
      const first = store.remove('2');
      const second = store.remove('2');

      http.expectOne('/api/posts/2').flush(POSTS[1]);
      await Promise.all([first, second]);

      expect(store.total()).toBe(2);
    });

    it('keeps the post and reports the error when the API fails', async () => {
      const removing = store.remove('2');
      http
        .expectOne('/api/posts/2')
        .flush({ error: { message: 'Post not found' } }, { status: 404, statusText: 'Not Found' });
      await removing;

      expect(store.total()).toBe(3);
      expect(store.error()).toBe('Post not found');
      expect(store.isPending('2')).toBe(false);
    });
  });

  describe('workflow', () => {
    beforeEach(() => load());

    it('offers only the transitions allowed from the current status', () => {
      expect(store.transitionsFor(post('x', 'x', { status: 'draft' })).map((t) => t.name)).toEqual(['publish']);
      expect(store.transitionsFor(post('x', 'x', { status: 'published' })).map((t) => t.name)).toEqual([
        'archive',
      ]);
      expect(store.transitionsFor(post('x', 'x', { status: 'archived' }))).toEqual([]);
    });

    it('replaces the post with the one returned after a transition', async () => {
      const applying = store.applyTransition('1', 'publish');

      const request = http.expectOne({ method: 'POST', url: '/api/posts/1/transitions' });
      expect(request.request.body).toEqual({ transition: 'publish' });
      request.flush({ ...POSTS[0], status: 'published' });
      await applying;

      expect(store.filteredPosts().map((p) => p.status)).toEqual(['published', 'draft', 'draft']);
    });
  });

  describe('attachments', () => {
    beforeEach(() => load());

    it('uploads the file and adds it to its post', async () => {
      const file = new File(['contenido'], 'brief.pdf', { type: 'application/pdf' });
      const attaching = store.attach('3', file);

      const request = http.expectOne({ method: 'POST', url: '/api/posts/3/attachments' });
      expect((request.request.body as FormData).get('file')).toBe(file);
      request.flush({
        id: 'a1',
        fileName: 'brief.pdf',
        mimeType: 'application/pdf',
        driveFileId: 'd1',
        webViewLink: 'https://drive.example/d1',
        createdAt: '2026-01-02T00:00:00Z',
      });
      await attaching;

      const [first, , third] = store.filteredPosts();
      expect(third.attachments.map((a) => a.fileName)).toEqual(['brief.pdf']);
      expect(first.attachments).toEqual([]);
    });
  });

  describe('brief extraction', () => {
    it('returns the suggestion from the API', async () => {
      const extracting = store.extractBrief('Campaña de invierno');

      const request = http.expectOne({ method: 'POST', url: '/api/briefs/extract' });
      expect(request.request.body).toEqual({ brief: 'Campaña de invierno' });
      request.flush({ name: 'Invierno', description: 'Resumen' });

      expect(await extracting).toEqual({ name: 'Invierno', description: 'Resumen' });
    });

    it('returns null and reports the error when Gemini fails', async () => {
      const extracting = store.extractBrief('x');
      http
        .expectOne('/api/briefs/extract')
        .flush({ error: { message: 'Gemini is overloaded' } }, { status: 502, statusText: 'Bad Gateway' });

      expect(await extracting).toBeNull();
      expect(store.error()).toBe('Gemini is overloaded');
    });
  });
});
