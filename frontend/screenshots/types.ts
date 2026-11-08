import type * as Go from "../bindings/github.com/myrheimb/pageroom/internal/app/app";
import type * as mock from "./mock";

export type Handler<Name extends keyof typeof Go> = (...args: Parameters<(typeof Go)[Name]>) => Promise<Awaited<ReturnType<(typeof Go)[Name]>>>;

// Fails to compile when the App API gains or renames a call the mock lacks.
export const complete: typeof mock extends { [Name in keyof typeof Go]: Handler<Name> } ? true : never = true;
