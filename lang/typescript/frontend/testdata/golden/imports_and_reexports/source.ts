import { A, B as C } from './local';
import type { T } from 'pkg';
import Default from './default';
import * as NS from './namespace';

export { A } from './local';
export * from './types';
export * as Bundle from './bundle';
export type { T } from 'pkg';
