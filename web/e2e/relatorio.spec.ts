import { abrir, criar, estado, expect, test } from "./base";

test("relatório pergunta, escreve com fontes, corrige uma seção e é aprovado", async ({ page, sistema }) => {
  await sistema.importarExemplos();
  await abrir(page, sistema);
  await page.getByRole("button", { name: /Relatório/ }).click();
  await page.getByLabel("Pesquisa profunda").check();
  await criar(page, "Relatório", `Condições do contrato de aluguel [perguntar]. Veja ${sistema.web}/pagina`);

  const perguntas = page.getByRole("form", { name: "Perguntas" });
  await perguntas.getByLabel("Qual período").fill("de 2026 a 2029");
  await perguntas.getByRole("button", { name: "Responder e continuar" }).click();

  const contexto = page.getByRole("region", { name: "Contexto" });
  const atencao = page.getByRole("region", { name: "Pontos de atenção" });
  await expect(estado(page)).toHaveText("pronta");
  await expect(contexto).toContainText("Segundo a fonte");
  await expect(atencao).toContainText("Segundo a fonte");
  // Toda afirmação tem ao menos uma fonte clicável.
  for (const p of await contexto.locator("p", { hasText: "Segundo a fonte" }).all()) {
    await expect(p.locator(".citacao")).not.toHaveCount(0);
  }
  await contexto.getByRole("button", { name: "F1" }).first().click();
  const fonte = page.getByRole("complementary", { name: "Fonte" });
  await expect(fonte).toContainText("/pagina");
  await fonte.page().locator('[data-secao="1"] .citacao').nth(1).click();
  await expect(fonte.getByRole("link", { name: "abrir no acervo" })).toBeVisible();

  const antes = await atencao.locator("p").allTextContents();
  await contexto.getByRole("textbox", { name: "Correção para Contexto" }).fill("seja mais curto");
  await contexto.getByRole("button", { name: "Corrigir" }).click();
  await expect(contexto).toContainText("Revisado (seja mais curto)");
  await expect(contexto).toContainText("Correção pedida: seja mais curto (atendida)");
  await expect(estado(page)).toHaveText("pronta");
  expect(await atencao.locator("p").allTextContents()).toEqual(antes);
  await contexto.getByRole("button", { name: "Corrigir" }).click();

  await contexto.getByRole("button", { name: "Aprovar seção" }).click();
  await expect(contexto.getByRole("button", { name: "Aprovada ✓" })).toBeVisible();
  await contexto.getByRole("button", { name: "Aprovada ✓" }).click();
  await expect(contexto.getByRole("button", { name: "Aprovar seção" })).toBeVisible();

  await page.getByRole("button", { name: "Aprovar e guardar no acervo" }).click();
  await expect(page.getByRole("log")).toContainText("Versão guardada no acervo");
  await page.getByRole("link", { name: /Versão 1/ }).click();
  await expect(page.getByRole("heading", { level: 1 })).toContainText("versão 1.md");
  await expect(page.getByText("## Fontes")).toBeVisible();
});

test("endereço bloqueado ou inacessível vira aviso na seção", async ({ page, sistema }) => {
  await abrir(page, sistema);
  await criar(page, "Relatório", `Prazos. Veja ${sistema.web}/nao-existe`);
  await expect(estado(page)).toHaveText("pronta");
  const contexto = page.getByRole("region", { name: "Contexto" });
  await expect(contexto).toContainText("não foi possível ler");
  await expect(contexto).toContainText("Nenhuma fonte sustentou esta seção");
  await expect(page.getByText("Clique numa marca")).toBeVisible();
});
