@Entity({ name: 'users' })
export class User {
  @Column({ type: 'varchar', length: 200 })
  name!: string;

  @ApiResponse({ status: 200 })
  @ApiResponse({ status: 404 })
  handle(@Inject('TOK') dep: Dep): void {}
}
