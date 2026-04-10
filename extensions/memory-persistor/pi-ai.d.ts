declare module '@mariozechner/pi-ai' {
  export interface TextContent {
    type: 'text';
    text: string;
  }

  export interface ImageContent {
    type: 'image';
    image?: string;
    url?: string;
    mimeType?: string;
  }
}
