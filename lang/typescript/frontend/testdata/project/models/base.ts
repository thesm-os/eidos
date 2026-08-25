import type { Identifiable } from '../src/user';

export abstract class Base implements Identifiable {
  abstract readonly id: string;
}
