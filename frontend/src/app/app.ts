import { ChangeDetectionStrategy, Component, OnInit, effect, inject } from '@angular/core';
import { MatCardModule } from '@angular/material/card';
import { MatSnackBar } from '@angular/material/snack-bar';
import { MatToolbarModule } from '@angular/material/toolbar';
import { PostFilter } from './posts/post-filter';
import { PostForm } from './posts/post-form';
import { PostList } from './posts/post-list';
import { PostsStore } from './posts/posts-store';

@Component({
  selector: 'app-root',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatCardModule, MatToolbarModule, PostFilter, PostForm, PostList],
  template: `
    <mat-toolbar class="app-bar">
      <h1>Posts</h1>
    </mat-toolbar>

    <main>
      <mat-card class="list-pane" appearance="outlined">
        <header class="pane-header">
          <div>
            <h2>Listado</h2>
            <p aria-live="polite">{{ summary() }}</p>
          </div>
          <app-post-filter />
        </header>
        <app-post-list />
      </mat-card>

      <mat-card class="form-pane" appearance="filled">
        <header class="pane-header">
          <div>
            <h2>Nuevo post</h2>
            <p>Completa el nombre y la descripción.</p>
          </div>
        </header>
        <app-post-form />
      </mat-card>
    </main>
  `,
  styles: `
    .app-bar {
      position: sticky;
      top: 0;
      z-index: 2;
      background: var(--mat-sys-surface-container);
      --mat-toolbar-container-text-color: var(--mat-sys-on-surface);
    }

    h1 {
      margin: 0;
      font: var(--mat-sys-title-large);
    }

    /* Compact: one column with 16px margins. */
    main {
      display: grid;
      grid-template-columns: minmax(0, 1fr);
      gap: 16px;
      max-width: 1200px;
      margin: 0 auto;
      padding: 16px;
    }

    .list-pane {
      overflow: hidden;
      background: var(--mat-sys-surface);
    }

    .form-pane {
      background: var(--mat-sys-surface-container-high);
    }

    .pane-header {
      display: flex;
      flex-wrap: wrap;
      align-items: center;
      justify-content: space-between;
      gap: 16px;
      padding: 16px 16px 12px;
    }

    .pane-header h2 {
      margin: 0;
      font: var(--mat-sys-title-medium);
    }

    .pane-header p {
      margin: 2px 0 0;
      font: var(--mat-sys-body-small);
      color: var(--mat-sys-on-surface-variant);
    }

    /* Full width under the title on compact; beside it from medium up. */
    app-post-filter {
      flex: 1 1 100%;
      min-width: 0;
    }

    app-post-form {
      padding: 0 16px 16px;
    }

    /* Medium: wider margins. */
    @media (min-width: 600px) {
      main {
        gap: 24px;
        padding: 24px;
      }

      .pane-header {
        padding: 20px 24px 12px;
      }

      app-post-filter {
        flex: 1 1 320px;
        max-width: 480px;
      }

      app-post-form {
        padding: 0 24px 24px;
      }
    }

    /* Expanded: list and form side by side; the form stays in view. */
    @media (min-width: 840px) {
      main {
        grid-template-columns: minmax(0, 1fr) 360px;
        align-items: start;
      }

      .form-pane {
        position: sticky;
        top: 88px;
      }
    }
  `,
})
export class App implements OnInit {
  private readonly store = inject(PostsStore);
  private readonly snackBar = inject(MatSnackBar);

  constructor() {
    // Every failed operation surfaces as a snackbar, wherever it started.
    effect(() => {
      const message = this.store.error();
      if (!message) return;
      this.snackBar.open(message, 'Cerrar', { duration: 6000 });
      this.store.error.set(null);
    });
  }

  ngOnInit(): void {
    // The only place the full list is requested: once per page load.
    this.store.load();
  }

  protected summary(): string {
    if (this.store.status() !== 'loaded') return ' ';
    const total = this.store.total();
    const shown = this.store.filteredPosts().length;
    const noun = total === 1 ? 'post' : 'posts';
    return this.store.filter().trim() ? `${shown} de ${total} ${noun}` : `${total} ${noun}`;
  }
}
