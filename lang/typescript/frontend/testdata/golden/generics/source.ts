export interface Container<T extends object = {}, K extends keyof T = keyof T> {
  get(key: K): T[K];
}

export class Impl<T> implements Container<object> {
  method<U extends T>(a: U): U {
    return a;
  }
}
