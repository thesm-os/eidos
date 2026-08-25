export interface User {
  readonly id: string;
  name?: string;
  tags: string[];
  greet(loud: boolean): Promise<string>;
}
