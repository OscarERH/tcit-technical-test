import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { MatButtonModule } from '@angular/material/button';
import { MatDialogModule, MatDialogRef } from '@angular/material/dialog';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatInputModule } from '@angular/material/input';
import { MatProgressBarModule } from '@angular/material/progress-bar';
import { NewPost } from './post.model';
import { PostsStore } from './posts-store';

/**
 * Asks for a campaign brief and closes with the name and description Gemini
 * extracted from it. The suggestion only pre-fills the form: nothing is saved
 * until the user reviews it and presses "Crear".
 */
@Component({
  selector: 'app-brief-dialog',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [MatButtonModule, MatDialogModule, MatFormFieldModule, MatInputModule, MatProgressBarModule],
  template: `
    <h2 mat-dialog-title>Completar desde un brief</h2>
    <mat-dialog-content>
      <p>
        Pega el brief de la campaña y Gemini propondrá el nombre y la descripción del post. El texto se
        envía a Gemini solo para esta extracción y no se guarda.
      </p>
      <mat-form-field appearance="outline">
        <mat-label>Brief de campaña</mat-label>
        <textarea
          matInput
          rows="8"
          maxlength="20000"
          [value]="brief()"
          [disabled]="loading()"
          (input)="brief.set($any($event.target).value)"
        ></textarea>
      </mat-form-field>
      @if (loading()) {
        <mat-progress-bar mode="indeterminate" />
      }
    </mat-dialog-content>
    <mat-dialog-actions align="end">
      <button matButton type="button" mat-dialog-close>Cancelar</button>
      <button matButton="filled" type="button" [disabled]="!brief().trim() || loading()" (click)="extract()">
        Extraer
      </button>
    </mat-dialog-actions>
  `,
  styles: `
    mat-dialog-content {
      width: min(560px, 80vw);
    }

    mat-form-field {
      width: 100%;
    }
  `,
})
export class BriefDialog {
  private readonly store = inject(PostsStore);
  private readonly dialogRef = inject<MatDialogRef<BriefDialog, NewPost>>(MatDialogRef);

  protected readonly brief = signal('');
  protected readonly loading = signal(false);

  protected async extract(): Promise<void> {
    this.loading.set(true);
    const suggestion = await this.store.extractBrief(this.brief());
    this.loading.set(false);

    // On failure the store reports the error and the dialog stays open to retry.
    if (suggestion) this.dialogRef.close(suggestion);
  }
}
