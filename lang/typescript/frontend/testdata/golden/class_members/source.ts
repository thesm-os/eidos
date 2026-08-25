export abstract class Repo<T> extends Base implements Store {
  private static readonly cache: Map<string, T> = new Map();
  #secret = 1;
  protected items!: T[];

  constructor(public readonly name: string, private dep?: Dep) {
    super();
  }

  abstract find(id: string): T | null;

  async *stream(): AsyncGenerator<T> {}

  get size(): number {
    return 0;
  }
}
