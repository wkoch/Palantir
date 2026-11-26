import { readdir, readFile } from "node:fs/promises";
import { join } from "node:path";
import { abrir, expect, test } from "./base";

test("copiar para um pendrive pede o destino e a confirmação", async ({ page, sistema }) => {
  await sistema.importarExemplos();
  await abrir(page, sistema, "#/acervo");
  await page.getByRole("button", { name: "Copiar para um disco ou pendrive" }).click();
  const painel = page.getByRole("region", { name: "Cópia para um disco" });
  const copiar = painel.getByRole("button", { name: "Copiar", exact: true });
  await expect(copiar).toBeDisabled();
  await painel.getByRole("radio", { name: /PENDRIVE/ }).check();
  await copiar.click();
  const confirmacao = painel.getByRole("alertdialog");
  await expect(confirmacao).toContainText("/dev/teste1");

  // Cancelar não grava nada.
  await confirmacao.getByRole("button", { name: "Cancelar" }).click();
  await expect(confirmacao).toBeHidden();
  expect(await readdir(sistema.pendrive)).toEqual([]);

  await copiar.click();
  await confirmacao.getByRole("button", { name: "Confirmar a cópia" }).click();
  await expect(painel.getByRole("status")).toContainText("Cópia concluída");
  const arquivos = await readdir(join(sistema.pendrive, "Palantir-copias"));
  expect(arquivos).toHaveLength(1);
  expect(arquivos[0]).toMatch(/^palantir-.*\.zip$/);
  const zip = await readFile(join(sistema.pendrive, "Palantir-copias", arquivos[0]!));
  for (const nome of ["banco/palantir.db", "config/config.toml", "diario/diario.jsonl", "metadados.json"]) {
    expect(zip.includes(nome)).toBe(true);
  }
  expect(zip.includes(sistema.chave)).toBe(false);

  // Sem discos, a tela diz o que fazer.
  await sistema.definirDiscos([]);
  await painel.getByRole("button", { name: "Procurar discos de novo" }).click();
  await expect(painel).toContainText("Conecte um pendrive");
});

test("uma cópia que falha é informada", async ({ page, sistema }) => {
  await sistema.definirDiscos([{ path: "/dev/teste", type: "disk", tran: "usb", children: [
    { path: "/dev/teste1", fstype: "vfat", label: "SOLTO", type: "part", size: 1024, mountpoints: [] },
  ] }]);
  await abrir(page, sistema, "#/acervo");
  await page.getByRole("button", { name: "Copiar para um disco ou pendrive" }).click();
  const painel = page.getByRole("region", { name: "Cópia para um disco" });
  await painel.getByRole("radio", { name: /SOLTO/ }).check();
  await painel.getByRole("button", { name: "Copiar", exact: true }).click();
  await painel.getByRole("button", { name: "Confirmar a cópia" }).click();
  await expect(painel.getByRole("status")).toContainText("falhou");
  await expect(painel.getByRole("status")).toContainText("montar o disco");
});
