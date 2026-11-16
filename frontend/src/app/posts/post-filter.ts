import { ChangeDetectionStrategy, Component, inject, linkedSignal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { PostsStore } from './posts-store';

@Component({
  selector: 'app-post-filter',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatFormFieldModule, MatIconModule, MatInputModule],
  template: `
    <form role="search" (submit)="search($event)">
      <mat-form-field appearance="outline" subscriptSizing="dynamic">
        <mat-label>Filtro de Nombre</mat-label>
        <mat-icon matPrefix svgIcon="search" />
        <input
          matInput
          type="text"
          enterkeyhint="search"
          autocomplete="off"
          [value]="term()"
          (input)="onInput($any($event.target).value)"
        />
        @if (term()) {
          <button matSuffix matIconButton type="button" aria-label="Limpiar filtro" (click)="clear()">
            <mat-icon svgIcon="close" />
          </button>
        }
      </mat-form-field>
      <button matButton="tonal" type="submit">Buscar</button>
    </form>
  `,
  styles: `
    :host {
      display: block;
    }

    form {
      display: flex;
      align-items: center;
      gap: 8px;
    }

    mat-form-field {
      flex: 1;
      min-width: 0;
    }
  `,
})
export class PostFilter {
  private readonly store = inject(PostsStore);

  /** Text being typed; follows the store when the filter is cleared elsewhere. */
  protected readonly term = linkedSignal(() => this.store.filter());

  protected search(event: Event): void {
    event.preventDefault();
    this.store.setFilter(this.term());
  }

  protected onInput(value: string): void {
    this.term.set(value);
    // Emptying the input removes the filter without having to press "Buscar".
    if (value === '') this.store.setFilter('');
  }

  protected clear(): void {
    this.term.set('');
    this.store.setFilter('');
  }
}
