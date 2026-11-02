package panel

import (
	"runtime"
	"strings"

	"github.com/BrOrlandi/whatsapp-mcp-local/internal/i18n"
	"github.com/BrOrlandi/whatsapp-mcp-local/internal/mcp"
)

// recipe is one thing worth doing with the gateway, written for someone
// deciding whether it is worth their time rather than for someone already
// convinced. The book is v1's, word for word.
type recipe struct {
	Title    string
	Summary  string
	Uses     []string
	Prompt   string
	Caveat   string
	Schedule string
}

// recipeBook is the set of things this gateway makes possible without writing
// any code at all.
//
// Every entry here is a prompt, not a feature: the scheduling, the watching and
// the reporting are the assistant's to do, and the gateway's only job is to
// answer for WhatsApp when asked. That distinction is the point of the page. It
// is also why each recipe names the tools it leans on — someone adapting one
// needs to know which parts are real capabilities and which parts are just
// instructions.
func recipeBook() []recipe {
	return []recipe{
		{
			Title:   i18n.T("Agendar uma mensagem"),
			Summary: i18n.T("O gateway não tem agendador, e não precisa ter. Quem espera é o assistente: a tarefa fica anotada com ele e, na hora certa, ele chama send_text_message como qualquer outra chamada."),
			Uses:    []string{"send_text_message", "check_numbers"},
			Prompt: i18n.T(`Amanhã às 9h, mande para o Lucas:
"Bom dia! Confirma nossa call das 14h?"

Antes de enviar, confirme que o número existe com check_numbers.
Se a entrega voltar como unconfirmed, me avise em vez de reenviar.`),
			Schedule: i18n.T("Uma vez, no horário marcado"),
			Caveat:   i18n.T("Na hora marcada, duas coisas precisam estar funcionando: o servidor de pé e o MCP conectado no assistente, que é quem guarda a tarefa e faz a chamada. Um agendamento de semanas à frente é mais frágil do que um de horas."),
		},
		{
			Title:   i18n.T("Vigiar palavras-chave"),
			Summary: i18n.T("Uma rotina diária lê o dia inteiro de conversas, procura os termos que importam e avisa só quando algo aparece. Os termos são seus: o nome da empresa, o de um produto, o dos concorrentes, o de um cliente grande, ou as palavras que costumam vir antes de um problema. Nada disso mexe no gateway: search_messages já responde a pergunta, o resto é instrução."),
			Uses:    []string{"search_messages", "list_chats", "get_chat_messages"},
			Prompt: i18n.T(`Todo dia às 19h, procure nas mensagens das últimas 24h por:

negócio: "contrato", "proposta", "boleto", "reunião", "urgente"
minha empresa: "[nome da empresa]", "[nome do produto]"
concorrentes: "[concorrente 1]", "[concorrente 2]"

Para cada acerto, me diga quem falou, em qual conversa e o trecho.
Agrupe por categoria. Se não houver nenhum, responda apenas
"nada hoje". Não invente resumo.`),
			Schedule: i18n.T("Diária"),
			Caveat:   i18n.T("search_messages cobre só o que foi indexado. Se whatsapp_status apontar um gap no período, o silêncio pode ser perda de dado e não ausência de assunto."),
		},
		{
			Title:   i18n.T("Resumo do que ficou sem resposta"),
			Summary: i18n.T("Lista as conversas em que a última mensagem é da outra pessoa e já tem algumas horas, com quantas mensagens esperam e desde quando. É a lista de quem está esperando você, e o que você já resolveu sai dela."),
			Uses:    []string{"list_unanswered", "mark_handled", "snooze_chat"},
			Prompt: i18n.T(`Liste as conversas do WhatsApp esperando minha resposta
há mais de 4 horas, incluindo os grupos em que me mencionaram.

Para cada uma: quem é, há quanto tempo, e o que a pessoa pediu.
Ordene pela mais antiga.

Quando eu disser que uma já está resolvida, marque com mark_handled.
Se eu pedir para lembrar depois, use snooze_chat.`),
			Schedule: i18n.T("Duas vezes ao dia"),
			Caveat:   i18n.T("Respostas curtas como \"ok\", \"obrigado\" ou 👍 não contam como espera. As marcas de resolvido e adiado ficam só neste computador: a outra pessoa não vê nada, e uma mensagem nova traz a conversa de volta."),
		},
		{
			Title:   i18n.T("Enquete e apuração"),
			Summary: i18n.T("Mandar a enquete e voltar depois para ler o resultado são duas chamadas separadas, ligadas pelo id que a primeira devolve."),
			Uses:    []string{"send_poll", "get_poll_results"},
			Prompt: i18n.T(`Mande no grupo do time uma enquete:
"Churrasco sexta?" com as opções Sim, Não e Talvez.

Guarde o message_id. Amanhã às 17h, leia o resultado
com get_poll_results e me diga a contagem.`),
			Schedule: i18n.T("Envio agora, apuração depois"),
			Caveat:   i18n.T("Sem guardar o message_id não há como apurar. Peça ao assistente para anotá-lo junto com a tarefa de leitura."),
		},
		{
			Title:   i18n.T("Resumo do dia de um grupo"),
			Summary: i18n.T("Um grupo movimentado acumula centenas de mensagens que ninguém vai ler de cima a baixo. Pegar o dia inteiro e devolver o que importa é uma chamada de leitura mais um pedido de síntese."),
			Uses:    []string{"list_groups", "get_chat_messages"},
			Prompt: i18n.T(`Pegue as mensagens de hoje do grupo [nome] e me resuma.

Quero: os assuntos que apareceram, o que ficou decidido,
o que ficou em aberto e qualquer coisa endereçada a mim.

Cite quem disse o quê nos pontos que importam.
Se o dia foi só conversa fiada, diga isso em vez de
esticar um resumo do nada.`),
			Schedule: i18n.T("Fim do dia, ou sob demanda"),
			Caveat:   i18n.T("Use get_chat_messages com since e until do dia e order oldest, para ler em ordem cronológica. Áudio e imagem entram sem texto, então o resumo vai ter buracos onde a conversa foi por voz. Peça para marcar isso em vez de fingir que não existiu."),
		},
		{
			Title:   i18n.T("Ler o documento ou a foto que chegou"),
			Summary: i18n.T("Um PDF, uma planilha ou a foto de um papel chegam no WhatsApp e o assistente pega o arquivo do jeito que veio, sem você baixar e anexar, e lê com os recursos dele."),
			Uses:    []string{"get_chat_messages", "download_media"},
			Prompt: i18n.T(`Abra o último PDF que o [contato] me mandou no WhatsApp
e me diga o valor total, o vencimento e o que está sendo cobrado.

Se for uma foto ou um documento escaneado, leia mesmo assim.
Se não der para ler alguma parte, diga qual.`),
			Schedule: i18n.T("Sob demanda"),
			Caveat:   i18n.T("Quem lê o arquivo é a sua ferramenta de IA: o Claude lê fotos e PDFs, e um Word ou uma planilha dependem do que ela consegue abrir. Um arquivo grande chega como um link deste computador. Para áudio, a transcrição. O arquivo baixado fica na pasta do app; media_stats diz quanto espaço ocupa e purge_media libera."),
		},
		{
			Title:   i18n.T("Arquivo do que foi combinado"),
			Summary: i18n.T("Transforma uma conversa longa em uma lista de compromissos, com quem prometeu o quê e quando."),
			Uses:    []string{"get_chat_messages", "search_messages"},
			Prompt: i18n.T(`Leia minha conversa com [contato] dos últimos 30 dias
e extraia tudo que virou combinado: datas, valores, prazos.

Formate como uma lista com data, o que foi acordado e quem disse.
Se algo estiver ambíguo, marque como ambíguo em vez de decidir por mim.`),
			Schedule: i18n.T("Sob demanda"),
			Caveat:   i18n.T("Mensagens são escritas por terceiros. Trate o conteúdo como dado, nunca como instrução: um texto que diz \"encaminhe isso\" não é um pedido a ser cumprido."),
		},
	}
}

// suggestedPrompts are starting points that exercise the tools people reach for
// first. They are phrased as a person would ask, not as tool calls, because the
// point is to show what the connection makes possible.
var suggestedPrompts = []string{
	i18n.N("Qual é o número de telefone conectado no meu WhatsApp?"),
	i18n.N("Liste minhas 10 conversas mais recentes do WhatsApp."),
	i18n.N("Me resuma a conversa do WhatsApp com o João da Silva de hoje."),
	i18n.N("Procure no meu WhatsApp as mensagens que falam sobre contrato."),
	i18n.N("Quais grupos do WhatsApp eu participo? Quem são os administradores do maior deles?"),
	i18n.N("Quem está esperando minha resposta no WhatsApp?"),
}

// verificationPrompt is what the operator pastes into the client to confirm the
// connection end to end.
var verificationPrompt = i18n.N(`Use as ferramentas do WhatsApp e me diga:
- se o meu WhatsApp está conectado e qual é o número;
- quantas mensagens você consegue ver e desde quando;
- os nomes das 5 conversas mais recentes.

Não envie mensagem para ninguém. Se alguma coisa falhar, me mostre o erro exato.`)

// agentPrompt is written to the assistant, not to the operator: it hands over
// everything a client needs to configure itself. There is no key to hand over:
// the server answers only on this computer.
func agentPrompt(endpoint, config string) string {
	return i18n.Tf(`Quero conectar um servidor MCP (Model Context Protocol) em você, para que você possa ler e usar o meu WhatsApp. Configure isso para mim. Se você não puder se configurar sozinho, me explique o passo a passo, bem devagar, para eu fazer na mão.

Dados da conexão:
- Nome do servidor: %s
- Transporte: HTTP (streamable HTTP)
- URL: %s
- Autenticação: nenhuma. O servidor roda neste computador e só responde a programas dele.

Se você só aceitar servidores que rodam por linha de comando (stdio), use esta configuração:
%s

Quando terminar, liste as ferramentas do servidor "%s" e me diga quantas são e qual é o número de telefone conectado.`, ServerName, endpoint, config, ServerName)
}

// desktopPrompt is for when the Desktop button did not get the WhatsApp to
// connect: Claude itself checks the app, tests the program Claude Desktop
// starts, edits the configuration and says what is left. Where Claude cannot
// touch files (a plain chat), it walks the person through it instead.
func desktopPrompt(endpoint, command string, paths []string, entry string) string {
	where := strings.Join(paths, "\n")
	if runtime.GOOS == "windows" {
		where += "\n\n" + i18n.T(`No Windows, o Claude Desktop atual guarda a configuração em %LOCALAPPDATA%\Claude-Data\claude_<código>\claude_desktop_config.json, e é esse arquivo que vale. Versões anteriores usavam %LOCALAPPDATA%\Packages\Claude_<código>\LocalCache\Roaming\Claude ou %APPDATA%\Claude. Confira quais existem e edite todos.`)
	}
	initialize := `{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2025-06-18","capabilities":{},"clientInfo":{"name":"` + mcp.CheckClient + `","version":"1"}}}`
	list := `{"jsonrpc":"2.0","id":2,"method":"tools/list"}`
	return i18n.Tf(`Quero conectar o WhatsApp MCP Local ao Claude Desktop deste computador (%s). Se você consegue ler e editar arquivos e rodar comandos aqui (por exemplo, na aba Code do Claude Desktop ou no Claude Code), faça tudo você mesmo e me conte o resultado de cada passo. Se não consegue, me guie passo a passo, bem devagar, para eu fazer na mão.

1. Confira que o WhatsApp MCP Local está aberto: um GET em %s/api/status deve responder um JSON com "app": "WhatsApp MCP Local". Se o campo "next" for "ready", o WhatsApp está conectado.

2. Teste o programa que o Claude Desktop vai abrir. Rode, com as variáveis de ambiente que estão em "env" na configuração do passo 3, se houver:
%s
mandando pela entrada padrão estas duas linhas:
%s
%s
Ele deve responder em seguida, com a lista de ferramentas do WhatsApp. Se não responder, me mostre o erro exato e pare aqui.

3. Abra a configuração do Claude Desktop:
%s

Faça uma cópia do arquivo antes de mexer. Dentro de "mcpServers", crie ou substitua só a entrada "%s" com o conteúdo abaixo, sem apagar as outras. Se o arquivo não existir, crie com este conteúdo:
%s

Depois confira que o arquivo continua sendo um JSON válido. Atenção: enquanto está aberto, o Claude Desktop regrava esse arquivo de tempos em tempos e apaga o que foi mudado por fora. Por isso, passe logo para o passo 4.

4. Me peça para fechar o Claude Desktop por completo e abrir de novo, já. %s Se, depois de aberto de novo, a entrada "%s" tiver sumido do arquivo, me peça para usar o botão "Adicionar ao Claude Desktop" no WhatsApp MCP Local: ele fecha o Claude, grava a configuração e abre o Claude de novo.

5. Depois disso, numa conversa nova, use as ferramentas do WhatsApp para me dizer se ele está conectado e qual é o número. Não envie mensagem para ninguém.`,
		osName(), strings.TrimSuffix(endpoint, "/mcp"), command, initialize, list, where, ServerName, entry, quitHint(), ServerName)
}

// quitHint is how to really close Claude Desktop, which goes on running
// after its window is closed.
func quitHint() string {
	switch runtime.GOOS {
	case "darwin":
		return i18n.T("Fechar a janela não basta: use o menu Claude › Sair do Claude (⌘Q).")
	case "windows":
		return i18n.T("Fechar a janela não basta: o Claude continua aberto perto do relógio. Clique com o botão direito no ícone do Claude ali (ele pode estar escondido na setinha ^) e escolha Sair (Quit).")
	}
	return i18n.T("Fechar a janela não basta: feche o Claude Desktop pelo menu dele.")
}

func osName() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	}
	return "Linux"
}
