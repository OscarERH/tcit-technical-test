import { HttpClient } from '@angular/common/http';
import { Injectable, inject } from '@angular/core';
import { Observable } from 'rxjs';
import { AppConfig, Attachment, NewPost, Post } from './post.model';

/** Thin HTTP client for the Go API. Holds no state. */
@Injectable({ providedIn: 'root' })
export class PostsApi {
  private readonly http = inject(HttpClient);

  config(): Observable<AppConfig> {
    return this.http.get<AppConfig>('/api/config');
  }

  list(): Observable<Post[]> {
    return this.http.get<Post[]>('/api/posts');
  }

  create(post: NewPost): Observable<Post> {
    return this.http.post<Post>('/api/posts', post);
  }

  remove(id: string): Observable<Post> {
    return this.http.delete<Post>(`/api/posts/${id}`);
  }

  transition(id: string, transition: string): Observable<Post> {
    return this.http.post<Post>(`/api/posts/${id}/transitions`, { transition });
  }

  attach(id: string, file: File): Observable<Attachment> {
    const body = new FormData();
    body.append('file', file);
    return this.http.post<Attachment>(`/api/posts/${id}/attachments`, body);
  }

  extractBrief(brief: string): Observable<NewPost> {
    return this.http.post<NewPost>('/api/briefs/extract', { brief });
  }
}
