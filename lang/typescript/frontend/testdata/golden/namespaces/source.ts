export namespace Outer.Inner {
  export interface Nested {
    v: string;
  }
  export const K = 1;
}

declare global {
  interface Window {
    custom: number;
  }
}
