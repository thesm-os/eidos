export declare function ambient(a: string): void;

export function overloaded(a: string): void;
export function overloaded(a: number): void;

export function generic<T extends object = {}>(value: T): T {
  return value;
}

export const NAME: string = 'eidos', COUNT = 2;
export let mutable: number;
