import { abrir, criar, estado, expect, test } from "./base";

test("estudo: edital, material, cartões, questões e desempenho, mesmo sem chave", async ({ page, sistema }) => {
  await sistema.importarExemplos();
  await abrir(page, sistema);
  await page.getByRole("button", { name: /Estudo/ }).click();
  await page.getByLabel("Edital no acervo").selectOption({ label: "edital-exemplo.txt" });
  await page.getByLabel("Título").fill("Analista administrativo");
  await page.getByRole("button", { name: "Começar" }).click();
  await expect(estado(page)).toHaveText("pronta");

  await expect(page.getByRole("row", { name: /Direito Administrativo/ }).first()).toBeVisible();
  await expect(page.getByRole("row", { name: /Licitações e contratos/ })).toBeVisible();
  await page.getByRole("button", { name: "Ligar ao acervo novamente" }).click();
  await expect(page.getByRole("log")).toContainText("Assuntos ligados");

  await page.getByRole("tab", { name: /Cartões/ }).click();
  await expect(page.getByText("Ainda não há cartões")).toBeVisible();
  await page.getByRole("tab", { name: /Questões/ }).click();
  await expect(page.getByText("Ainda não há questões")).toBeVisible();
  await page.getByRole("tab", { name: "Assuntos" }).click();
  await page.getByRole("button", { name: "Gerar material de Direito Administrativo" }).click();
  await expect(page.getByRole("tab", { name: /Questões \([1-9]/ })).toBeVisible();
  await expect(estado(page)).toHaveText("pronta");

  // Sem chave, revisar e responder continuam funcionando.
  await page.getByRole("link", { name: "Ajustes" }).click();
  await page.locator("form", { has: page.locator("#segredo-openrouter") }).getByRole("button", { name: "Remover" }).click();
  await expect(page.locator('[data-teste="modelos"]')).toHaveText("sem chave");
  await page.goBack();

  await page.getByRole("tab", { name: /Cartões/ }).click();
  const contador = page.getByText(/para revisar ·/);
  const total = Number((await contador.textContent())!.split(" ")[0]);
  expect(total).toBeGreaterThan(1);
  await page.locator("body").press("1"); // sem virar, a nota não vale
  await expect(contador).toContainText(`${total} para revisar`);
  await page.locator("body").press(" ");
  await expect(page.locator(".flash .verso")).toBeVisible();
  await page.getByText(/^Fonte: /).click();
  await expect(page.getByRole("link", { name: "abrir no acervo" })).toBeVisible();
  await page.locator("body").press("3");
  await expect(contador).toContainText(`${total - 1} para revisar`);
  await expect(page.locator(".flash .verso")).toBeHidden();
  for (let restantes = total - 1; restantes > 0; restantes--) {
    await page.getByRole("button", { name: /Mostrar resposta/ }).click();
    await page.getByRole("group", { name: "Como foi?" }).getByRole("button", { name: restantes === 1 ? /Fácil/ : /Bom/ }).click();
    if (restantes > 1) await expect(contador).toContainText(`${restantes - 1} para revisar`);
  }
  await expect(page.getByText("Nada para revisar agora")).toBeVisible();
  await expect(page.getByText("Próxima revisão:")).toBeVisible();

  await page.getByRole("tab", { name: /Questões/ }).click();
  const responder = page.getByRole("button", { name: "Responder" });
  await expect(responder).toBeDisabled();
  const primeira = await page.locator(".alternativas > p").nth(1).textContent();
  await page.getByRole("radio").nth(1).check(); // a correta no simulador
  await responder.click();
  await expect(page.getByRole("status")).toContainText("Certo!");
  await page.getByRole("button", { name: "Próxima questão" }).click();
  await expect(page.locator(".alternativas > p").nth(1)).not.toHaveText(primeira!);
  await page.getByRole("radio").nth(0).check();
  await responder.click();
  await expect(page.getByRole("status")).toContainText("Errado. A correta é B.");
  await expect(page.locator("label.errada")).toHaveCount(1);

  await page.getByRole("tab", { name: "Desempenho" }).click();
  await expect(page.getByText("Aproveitamento")).toBeVisible();
  await expect(page.getByText("Ponto mais fraco:")).toBeVisible();
});

test("estudo com o conteúdo colado e assunto sem material", async ({ page, sistema }) => {
  await abrir(page, sistema);
  await criar(page, "Estudo", "Informática: Redes; Segurança");
  await expect(estado(page)).toHaveText("pronta");
  await page.getByRole("button", { name: "Gerar material de Redes" }).click();
  await expect(page.getByRole("row", { name: /Redes/ })).toContainText("Não há material no acervo");
});
