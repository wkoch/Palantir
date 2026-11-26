import { resolve } from "node:path";

export const web = resolve(import.meta.dirname, "..");
export const raiz = resolve(web, "..");
export const binarios = resolve(web, "cobertura", "bin");
