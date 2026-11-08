export interface Message {
  id: string;
}

export type Id = string;

export enum Kind {
  A,
  B,
}

const sql = `
export class NotAClass {
function notAFunction() {
`;

export class Model {
  private items: Message[] = [];

  add(m: Message): void {
    this.items.push(m);
  }

  get size(): number {
    return this.items.length;
  }
}

export const card = <T>(v: T) => {
  return v;
};

export async function load(id: Id): Promise<Message> {
  return { id };
}
