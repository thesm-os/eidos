/**
 * A user of the system.
 *
 * @deprecated prefer Person
 */
@Entity({ name: 'users' })
export class User extends Base implements Identifiable {
  @Column({ type: 'varchar', length: 200 })
  readonly name!: string;

  age?: number;

  constructor(public readonly id: string) {
    super();
  }

  async greet(loud: boolean): Promise<string> {
    return this.name;
  }
}

export interface Identifiable {
  readonly id: string;
  describe(): string;
}
