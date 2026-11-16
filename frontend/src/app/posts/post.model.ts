export interface Attachment {
  id: string;
  fileName: string;
  mimeType: string;
  driveFileId: string;
  webViewLink: string;
  createdAt: string;
}

export interface Post {
  id: string;
  name: string;
  description: string;
  status: string;
  createdAt: string;
  attachments: Attachment[];
}

export type NewPost = Pick<Post, 'name' | 'description'>;

/** One edge of the post lifecycle, as defined by the Symfony workflow service. */
export interface Transition {
  name: string;
  from: string[];
  to: string;
}

export interface Features {
  attachments: boolean;
  briefExtraction: boolean;
  workflow: boolean;
}

export interface AppConfig {
  features: Features;
  transitions: Transition[];
}

export const NAME_MAX_LENGTH = 100;
export const DESCRIPTION_MAX_LENGTH = 500;
