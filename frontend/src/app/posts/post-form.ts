import { TextFieldModule } from '@angular/cdk/text-field';
import { ChangeDetectionStrategy, Component, inject, signal } from '@angular/core';
import { FormBuilder, FormGroupDirective, ReactiveFormsModule, Validators } from '@angular/forms';
import { MatButtonModule } from '@angular/material/button';
import { MatDialog } from '@angular/material/dialog';
import { MatFormFieldModule } from '@angular/material/form-field';
import { MatIconModule } from '@angular/material/icon';
import { MatInputModule } from '@angular/material/input';
import { firstValueFrom } from 'rxjs';
import { BriefDialog } from './brief-dialog';
import { DESCRIPTION_MAX_LENGTH, NAME_MAX_LENGTH, NewPost } from './post.model';
import { PostsStore } from './posts-store';

/** Rejects values made only of whitespace, which the API would refuse. */
const notBlank = Validators.pattern(/\S/);

@Component({
  selector: 'app-post-form',
  changeDetection: ChangeDetectionStrategy.OnPush,
  imports: [
    ReactiveFormsModule,
    TextFieldModule,
    MatButtonModule,
    MatFormFieldModule,
    MatIconModule,
    MatInputModule,
  ],
  template: `
    <form [formGroup]="form" #formDirective="ngForm" (ngSubmit)="submit(formDirective)">
      <mat-form-field appearance="outline">
        <mat-label>Nombre</mat-label>
        <input matInput formControlName="name" autocomplete="off" [maxlength]="nameMaxLength" />
        <mat-hint align="end">{{ form.controls.name.value.length }} / {{ nameMaxLength }}</mat-hint>
        <mat-error>Ingresa un nombre.</mat-error>
      </mat-form-field>

      <mat-form-field appearance="outline">
        <mat-label>Descripción</mat-label>
        <textarea
          matInput
          formControlName="description"
          cdkTextareaAutosize
          cdkAutosizeMinRows="3"
          cdkAutosizeMaxRows="8"
          [maxlength]="descriptionMaxLength"
        ></textarea>
        <mat-hint align="end">
          {{ form.controls.description.value.length }} / {{ descriptionMaxLength }}
        </mat-hint>
        <mat-error>Ingresa una descripción.</mat-error>
      </mat-form-field>

      <div class="actions">
        @if (store.features().briefExtraction) {
          <button matButton type="button" (click)="fillFromBrief()">
            <mat-icon svgIcon="auto_awesome" />
            Completar con IA
          </button>
        }
        <button matButton="filled" type="submit" [disabled]="form.invalid || submitting()">
          <mat-icon svgIcon="add" />
          Crear
        </button>
      </div>
    </form>
  `,
  styles: `
    :host {
      display: block;
    }

    form {
      display: flex;
      flex-direction: column;
      gap: 8px;
    }

    .actions {
      display: flex;
      flex-wrap: wrap;
      align-items: center;
      justify-content: space-between;
      gap: 8px;
      margin-top: 4px;
    }

    /* The primary action stays on the right even when it is alone. */
    .actions button[type='submit'] {
      margin-left: auto;
    }
  `,
})
export class PostForm {
  protected readonly store = inject(PostsStore);
  private readonly dialog = inject(MatDialog);

  protected readonly nameMaxLength = NAME_MAX_LENGTH;
  protected readonly descriptionMaxLength = DESCRIPTION_MAX_LENGTH;
  protected readonly submitting = signal(false);

  protected readonly form = inject(FormBuilder).nonNullable.group({
    name: ['', [Validators.required, notBlank, Validators.maxLength(NAME_MAX_LENGTH)]],
    description: ['', [Validators.required, notBlank, Validators.maxLength(DESCRIPTION_MAX_LENGTH)]],
  });

  protected async submit(formDirective: FormGroupDirective): Promise<void> {
    if (this.form.invalid || this.submitting()) return;

    this.submitting.set(true);
    const created = await this.store.create(this.form.getRawValue());
    this.submitting.set(false);

    // Keep the typed values when the request failed so the user can retry.
    // Resetting through the directive also clears the "submitted" error state.
    if (created) formDirective.resetForm();
  }

  /** Opens the brief dialog and pre-fills the form with Gemini's suggestion. */
  protected async fillFromBrief(): Promise<void> {
    const suggestion: NewPost | undefined = await firstValueFrom(this.dialog.open(BriefDialog).afterClosed());
    if (suggestion) this.form.setValue(suggestion);
  }
}
