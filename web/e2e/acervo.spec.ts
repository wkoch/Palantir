import { readFile, writeFile } from "node:fs/promises";
import { join } from "node:path";
import { abrir, expect, test } from "./base";

test("importar por arquivo, arrastando, por pasta e pela pasta de entrada", async ({ page, sistema }) => {
  await abrir(page, sistema, "#/acervo");
  await expect(page.getByText("Nenhum documento")).toBeVisible();

  await page.getByLabel("Arquivos (ou arraste").setInputFiles(sistema.exemplo("contrato-de-aluguel.txt"));
  await expect(page.getByRole("log")).toContainText("1 importado(s)");
  await expect(page.getByRole("link", { name: /contrato-de-aluguel/ })).toContainText("pronto");

  const dados = await page.evaluateHandle(() => {
    const t = new DataTransfer();
    t.items.add(new File(["Nota arrastada sobre o seguro do carro."], "seguro.txt", { type: "text/plain" }));
    t.items.add(new File(["\0\0\0binario"], "estranho.bin"));
    return t;
  });
  await page.dispatchEvent("body", "dragover", { dataTransfer: dados });
  await expect(page.getByText("Solte para importar no acervo")).toBeVisible();
  await page.dispatchEvent("body", "dragleave", { dataTransfer: dados });
  await expect(page.getByText("Solte para importar no acervo")).toBeHidden();
  await page.dispatchEvent("body", "dragover", { dataTransfer: dados });
  await page.dispatchEvent("body", "drop", { dataTransfer: dados });
  await expect(page.getByRole("link", { name: /seguro\.txt/ })).toBeVisible();
  await expect(page.getByRole("link", { name: /estranho\.bin/ })).toContainText("erro");

  await page.getByLabel("Pasta ou arquivo deste computador").fill(join(sistema.pasta, "exemplos"));
  await page.getByRole("button", { name: "Importar", exact: true }).click();
  await expect(page.getByRole("log")).toContainText("2 importado(s), 1 já no acervo");
  await page.getByLabel("Pasta ou arquivo deste computador").fill(join(sistema.pasta, "config"));
  await page.getByRole("button", { name: "Importar", exact: true }).click();
  await expect(page.getByRole("log")).toContainText("pertence ao próprio Palantír");

  await writeFile(join(sistema.pasta, "Entrada", "recibo.txt"), "Recibo de exemplo: R$ 40,00 em 02/11/2026.");
  await expect(page.getByRole("link", { name: /recibo\.txt/ })).toBeVisible({ timeout: 20_000 });
});

test("buscar, organizar, conferir, regenerar e exportar", async ({ page, sistema }) => {
  await sistema.importarExemplos();
  await abrir(page, sistema, "#/acervo");

  const busca = page.getByRole("searchbox", { name: "Procurar no acervo" });
  await busca.fill("multa aluguel");
  await busca.press("Enter");
  await expect(page.getByRole("heading", { name: /Resultados/ })).toBeVisible();
  await expect(page.getByRole("article").first()).toContainText("contrato-de-aluguel.txt");
  await page.getByRole("button", { name: "Limpar busca" }).click();
  await busca.fill("");
  await busca.press("Enter");
  await expect(page.getByRole("heading", { name: "Documentos" })).toBeVisible();
  await busca.fill("zzzzqqqq");
  await busca.press("Enter");
  await expect(page.getByText("Nada encontrado.")).toBeVisible();
  await page.getByRole("button", { name: "Limpar busca" }).click();

  // Coleções e entidades criadas pelo modelo filtram a lista.
  await page.getByRole("button", { name: /Contratos \(\d\)/ }).click();
  await expect(page.getByRole("button", { name: "Mostrar todos" })).toBeVisible();
  await expect(page.getByRole("link", { name: /contrato-de-aluguel/ })).toBeVisible();
  await page.getByRole("button", { name: "Mostrar todos" }).click();
  await page.getByRole("button", { name: "R$ 1.850,00" }).click();
  await expect(page.locator("a.item")).toHaveCount(1);
  await page.getByRole("button", { name: "Mostrar todos" }).click();

  await page.getByLabel("Nome da nova coleção").fill("Minha casa");
  await page.getByRole("button", { name: "Criar", exact: true }).click();
  await expect(page.getByRole("button", { name: "Minha casa (0)" })).toBeVisible();
  await page.getByRole("link", { name: /contrato-de-aluguel/ }).click();
  await page.getByLabel("Coleção", { exact: true }).selectOption({ label: "Minha casa" });
  await page.getByRole("button", { name: "Incluir na coleção" }).click();
  await expect(page.getByRole("log")).toContainText("Incluído na coleção");
  await expect(page.getByTitle("data").first()).toBeVisible();

  const [original] = await Promise.all([
    page.context().waitForEvent("page"),
    page.getByRole("button", { name: "Abrir o original" }).click(),
  ]);
  await expect(original.locator("body")).toContainText("CONTRATO DE ALUGUEL");
  await original.close();
  await page.getByRole("button", { name: "Extrair novamente" }).click();
  await expect(page.locator("span.selo.pronto")).toBeVisible();
  await page.getByRole("link", { name: "← Acervo" }).click();
  await expect(page.getByRole("button", { name: "Minha casa (1)" })).toBeVisible();

  await page.getByRole("button", { name: "Verificar originais" }).click();
  await expect(page.getByRole("log")).toContainText("3 original(is) íntegro(s)");
  const [regenerado] = await Promise.all([
    page.waitForResponse("**/api/acervo/regenerar"),
    page.getByRole("button", { name: "Regenerar derivados" }).click(),
  ]);
  expect(await regenerado.json()).toMatchObject({ conferidos: 3, problemas: [] });
  await expect(page.getByRole("button", { name: "Minha casa (1)" })).toBeVisible();

  const [baixado] = await Promise.all([
    page.waitForEvent("download"),
    page.getByRole("button", { name: "Exportar tudo" }).click(),
  ]);
  expect(baixado.suggestedFilename()).toMatch(/^palantir-\d{4}-\d{2}-\d{2}\.zip$/);
  const zip = await readFile(await baixado.path());
  expect(zip.subarray(0, 2).toString()).toBe("PK");
  expect(zip.includes("metadados.json")).toBe(true);
});
