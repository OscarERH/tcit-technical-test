import { HttpErrorResponse } from '@angular/common/http';
import { Injectable, computed, inject, signal } from '@angular/core';
import { firstValueFrom } from 'rxjs';
import { AppConfig, NewPost, Post, Transition } from './post.model';
import { PostsApi } from './posts-api';

type Status = 'idle' | 'loading' | 'loaded' | 'failed';

const NO_CONFIG: AppConfig = {
  features: { attachments: false, briefExtraction: false, workflow: false },
  transitions: [],
};

/**
 * Single source of truth for the posts view.
 *
 * The full list is requested once per page load; from then on every change
 * (create, delete, transition, attachment) is applied locally from the API's
 * response, and the name filter runs over the posts already in memory.
 */
@Injectable({ providedIn: 'root' })
export class PostsStore {
  private readonly api = inject(PostsApi);

  private readonly posts = signal<Post[]>([]);
  private readonly config = signal<AppConfig>(NO_CONFIG);
  private readonly pending = signal<ReadonlySet<string>>(new Set());

  readonly status = signal<Status>('idle');
  readonly filter = signal('');
  readonly error = signal<string | null>(null);

  readonly features = computed(() => this.config().features);
  readonly total = computed(() => this.posts().length);

  readonly filteredPosts = computed(() => {
    const term = this.filter().trim().toLocaleLowerCase();
    const posts = this.posts();
    return term ? posts.filter((post) => post.name.toLocaleLowerCase().includes(term)) : posts;
  });

  /** Loads the list once; later calls are ignored. */
  async load(): Promise<void> {
    if (this.status() !== 'idle') return;
    this.status.set('loading');

    // Optional integrations must never block the list.
    firstValueFrom(this.api.config())
      .then((config) => this.config.set(config))
      .catch(() => this.config.set(NO_CONFIG));

    try {
      this.posts.set(await firstValueFrom(this.api.list()));
      this.status.set('loaded');
    } catch (error) {
      this.status.set('failed');
      this.fail(error);
    }
  }

  setFilter(term: string): void {
    this.filter.set(term);
  }

  /** Returns whether the post was created, so the form knows when to reset. */
  async create(post: NewPost): Promise<boolean> {
    try {
      const created = await firstValueFrom(this.api.create(post));
      this.posts.update((posts) => [...posts, created]);
      return true;
    } catch (error) {
      this.fail(error);
      return false;
    }
  }

  remove(id: string): Promise<void> {
    return this.withPending(id, async () => {
      const deleted = await firstValueFrom(this.api.remove(id));
      this.posts.update((posts) => posts.filter((post) => post.id !== deleted.id));
    });
  }

  applyTransition(id: string, transition: string): Promise<void> {
    return this.withPending(id, async () => {
      const updated = await firstValueFrom(this.api.transition(id, transition));
      this.replace(updated.id, () => updated);
    });
  }

  attach(id: string, file: File): Promise<void> {
    return this.withPending(id, async () => {
      const attachment = await firstValueFrom(this.api.attach(id, file));
      this.replace(id, (post) => ({ ...post, attachments: [...post.attachments, attachment] }));
    });
  }

  async extractBrief(brief: string): Promise<NewPost | null> {
    try {
      return await firstValueFrom(this.api.extractBrief(brief));
    } catch (error) {
      this.fail(error);
      return null;
    }
  }

  /** Transitions the workflow allows from the post's current status. */
  transitionsFor(post: Post): Transition[] {
    return this.config().transitions.filter((transition) => transition.from.includes(post.status));
  }

  isPending(id: string): boolean {
    return this.pending().has(id);
  }

  private replace(id: string, change: (post: Post) => Post): void {
    this.posts.update((posts) => posts.map((post) => (post.id === id ? change(post) : post)));
  }

  /** Runs a row action while marking that row as busy. */
  private async withPending(id: string, action: () => Promise<void>): Promise<void> {
    if (this.isPending(id)) return;
    this.pending.update((ids) => new Set(ids).add(id));
    try {
      await action();
    } catch (error) {
      this.fail(error);
    } finally {
      this.pending.update((ids) => {
        const next = new Set(ids);
        next.delete(id);
        return next;
      });
    }
  }

  private fail(error: unknown): void {
    const apiMessage = error instanceof HttpErrorResponse ? error.error?.error?.message : null;
    this.error.set(
      typeof apiMessage === 'string' ? apiMessage : 'No se pudo completar la operación. Intenta nuevamente.',
    );
  }
}
