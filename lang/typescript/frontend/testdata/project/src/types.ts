export type ID = string | number;
export type Maybe<T> = T | null;
export type Handler = (event: string, payload: unknown) => void;
export type Pair = [key: string, value: number];
export type Keys<T> = keyof T;

export enum Role {
  Admin = 'admin',
  Guest = 'guest',
}

export const DEFAULT_ROLE: Role = Role.Guest;
export let counter = 0;

export function identity<T>(value: T): T {
  return value;
}
