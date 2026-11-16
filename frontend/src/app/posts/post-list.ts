import { ChangeDetectionStrategy, Component, computed, inject } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatIconModule } from '@angular/material/icon';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { MatTableModule } from '@angular/material/table';
import { MatTooltipModule } from '@angular/material/tooltip';
import { Post } from './post.model';
import { PostsStore } from './posts-store';

const STATUS_LABELS: Record<string, string> = {
  draft: 'Borrador',
  published: 'Publicado',
  archived: 'Archivado',
};

const TRANSITION_LABELS: Record<string, string> = {
  publish: 'Publicar',
  archive: 'Archivar',
  restore: 'Restaurar',
};

@Component({
  selector: 'app-post-list',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatIconModule, MatProgressBarModule, MatTableModule, MatTooltipModule],
  template: `
    <div class="progress">
      @if (store.status() === 'loading') {
        <mat-progress-bar mode="indeterminate" aria-label="Cargando posts" />
      }
    </div>

    <div class="table-scroll">
      <table mat-table [dataSource]="store.filteredPosts()" [trackBy]="trackById">
        <ng-container matColumnDef="name">
          <th mat-header-cell *matHeaderCellDef>Nombre</th>
          <td mat-cell *matCellDef="let post">
            <span class="name">{{ post.name }}</span>
            <!-- Compact screens fold the description and the status under the name. -->
            <span class="description-inline">{{ post.description }}</span>
            @if (store.features().workflow) {
              <span class="status status-inline" [class]="'status-' + post.status">
                {{ statusLabel(post.status) }}
              </span>
            }
          </td>
        </ng-container>

        <ng-container matColumnDef="description">
          <th mat-header-cell *matHeaderCellDef>Descripción</th>
          <td mat-cell *matCellDef="let post">
            {{ post.description }}
            @if (post.attachments.length > 0) {
              <div class="attachments">
                @for (attachment of post.attachments; track attachment.id) {
                  <a [href]="attachment.webViewLink" target="_blank" rel="noopener noreferrer">
                    {{ attachment.fileName }}
                  </a>
                }
              </div>
            }
          </td>
        </ng-container>

        <ng-container matColumnDef="status">
          <th mat-header-cell *matHeaderCellDef>Estado</th>
          <td mat-cell *matCellDef="let post">
            <span class="status" [class]="'status-' + post.status">{{ statusLabel(post.status) }}</span>
          </td>
        </ng-container>

        <ng-container matColumnDef="actions">
          <th mat-header-cell *matHeaderCellDef>Acción</th>
          <td mat-cell *matCellDef="let post">
            <div class="actions">
              @for (transition of store.transitionsFor(post); track transition.name) {
                <button
                  matButton
                  type="button"
                  [disabled]="store.isPending(post.id)"
                  (click)="store.applyTransition(post.id, transition.name)"
                >
                  {{ transitionLabel(transition.name) }}
                </button>
              }
              @if (store.features().attachments) {
                <button
                  matIconButton
                  type="button"
                  matTooltip="Adjuntar archivo"
                  [attr.aria-label]="'Adjuntar archivo a ' + post.name"
                  [disabled]="store.isPending(post.id)"
                  (click)="fileInput.click()"
                >
                  <mat-icon svgIcon="attach_file" />
                </button>
                <input #fileInput type="file" hidden (change)="attach(post, fileInput)" />
              }
              <button
                matIconButton
                class="danger"
                type="button"
                matTooltip="Eliminar"
                [attr.aria-label]="'Eliminar ' + post.name"
                [disabled]="store.isPending(post.id)"
                (click)="store.remove(post.id)"
              >
                <mat-icon svgIcon="delete" />
              </button>
            </div>
          </td>
        </ng-container>

        <tr mat-header-row *matHeaderRowDef="columns()"></tr>
        <tr mat-row *matRowDef="let post; columns: columns()" [class.pending]="store.isPending(post.id)"></tr>

        <tr class="mat-mdc-row" *matNoDataRow>
          <td class="mat-mdc-cell" [attr.colspan]="columns().length">
            <div class="empty">
              <mat-icon [svgIcon]="filtering() ? 'search' : 'add'" />
              <p>{{ emptyMessage() }}</p>
              @if (filtering() && store.status() === 'loaded') {
                <button matButton="outlined" type="button" (click)="store.setFilter('')">Limpiar filtro</button>
              }
            </div>
          </td>
        </tr>
      </table>
    </div>
  `,
  styles: `
    :host {
      display: block;
      border-top: 1px solid var(--mat-sys-outline-variant);
    }

    /* Reserved height so the bar does not shift the table when it appears. */
    .progress {
      height: 4px;
      margin-top: -1px;
    }

    .table-scroll {
      overflow-x: auto;
    }

    table {
      --mat-table-background-color: transparent;
      --mat-table-row-item-outline-color: var(--mat-sys-outline-variant);
      --mat-table-header-headline-color: var(--mat-sys-on-surface-variant);
    }

    th {
      font: var(--mat-sys-label-large);
    }

    td {
      overflow-wrap: anywhere;
    }

    /* Hover state layer, as on any Material list item. */
    tr[mat-row] {
      transition:
        background-color 150ms ease,
        opacity 150ms ease;
    }

    tr[mat-row]:hover {
      background: color-mix(in srgb, var(--mat-sys-on-surface) 6%, transparent);
    }

    tr.pending {
      opacity: 0.5;
    }

    .mat-column-name {
      width: 26%;
      min-width: 120px;
    }

    .name {
      font: var(--mat-sys-title-small);
    }

    .description-inline {
      display: none;
      color: var(--mat-sys-on-surface-variant);
    }

    .mat-column-status {
      width: 112px;
    }

    .mat-column-actions {
      width: 1%;
      padding-right: 8px;
      white-space: nowrap;
    }

    .actions {
      display: flex;
      align-items: center;
      justify-content: flex-end;
    }

    th.mat-column-actions {
      text-align: right;
      padding-right: 20px;
    }

    .danger:not(:disabled) {
      --mat-icon-button-icon-color: var(--mat-sys-error);
      --mat-icon-button-state-layer-color: var(--mat-sys-error);
    }

    .status {
      display: inline-block;
      padding: 4px 10px;
      border-radius: var(--mat-sys-corner-small);
      font: var(--mat-sys-label-medium);
      white-space: nowrap;
      background: var(--mat-sys-surface-container-highest);
      color: var(--mat-sys-on-surface-variant);
    }

    .status-published {
      background: var(--mat-sys-primary-container);
      color: var(--mat-sys-on-primary-container);
    }

    .status-archived {
      background: transparent;
      outline: 1px solid var(--mat-sys-outline-variant);
      outline-offset: -1px;
    }

    .attachments {
      display: flex;
      flex-wrap: wrap;
      gap: 4px 12px;
      margin: 4px 0;
      font: var(--mat-sys-body-small);
    }

    .attachments a {
      color: var(--mat-sys-primary);
    }

    .empty {
      display: flex;
      flex-direction: column;
      align-items: center;
      gap: 8px;
      padding: 40px 16px;
      text-align: center;
      color: var(--mat-sys-on-surface-variant);
    }

    .empty mat-icon {
      width: 40px;
      height: 40px;
      opacity: 0.6;
    }

    .empty p {
      margin: 0;
      max-width: 44ch;
      font: var(--mat-sys-body-large);
    }

    .status.status-inline {
      display: none;
    }

    /* Compact: only name and actions remain as columns; the description and
       the status move under the name so nothing scrolls sideways. */
    @media (max-width: 599.98px) {
      .mat-column-description,
      .mat-column-status {
        display: none;
      }

      .mat-column-name {
        width: auto;
        min-width: 0;
        padding-block: 12px;
      }

      .name,
      .description-inline {
        display: block;
      }

      .status.status-inline {
        display: inline-block;
        margin-top: 6px;
      }
    }
  `,
})
export class PostList {
  protected readonly store = inject(PostsStore);

  /** The lifecycle column only exists when the workflow service is available. */
  protected readonly columns = computed(() =>
    this.store.features().workflow
      ? ['name', 'description', 'status', 'actions']
      : ['name', 'description', 'actions'],
  );

  protected readonly filtering = computed(() => this.store.filter().trim() !== '');

  protected readonly emptyMessage = computed(() => {
    switch (this.store.status()) {
      case 'idle':
      case 'loading':
        return 'Cargando posts…';
      case 'failed':
        return 'No se pudieron cargar los posts. Recarga la página para reintentar.';
      default:
        return this.filtering()
          ? `Ningún post tiene "${this.store.filter().trim()}" en su nombre. El filtro no busca en la descripción.`
          : 'Aún no hay posts. Crea el primero con el formulario.';
    }
  });

  protected readonly trackById = (_: number, post: Post) => post.id;

  protected statusLabel(status: string): string {
    return STATUS_LABELS[status] ?? status;
  }

  protected transitionLabel(name: string): string {
    return TRANSITION_LABELS[name] ?? name;
  }

  protected attach(post: Post, input: HTMLInputElement): void {
    const file = input.files?.[0];
    // Reset so choosing the same file again fires another change event.
    input.value = '';
    if (file) this.store.attach(post.id, file);
  }
}
