package panel

import "fmt"

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
			Title:   "Agendar uma mensagem",
			Summary: "O gateway não tem agendador, e não precisa ter. Quem espera é o assistente: a tarefa fica anotada com ele e, na hora certa, ele chama send_text_message como qualquer outra chamada.",
			Uses:    []string{"send_text_message", "check_numbers"},
			Prompt: `Amanhã às 9h, mande para o Lucas:
"Bom dia! Confirma nossa call das 14h?"

Antes de enviar, confirme que o número existe com check_numbers.
Se a entrega voltar como unconfirmed, me avise em vez de reenviar.`,
			Schedule: "Uma vez, no horário marcado",
			Caveat:   "Na hora marcada, duas coisas precisam estar funcionando: o servidor de pé e o MCP conectado no assistente, que é quem guarda a tarefa e faz a chamada. Um agendamento de semanas à frente é mais frágil do que um de horas.",
		},
		{
			Title:   "Vigiar palavras-chave",
			Summary: "Uma rotina diária lê o dia inteiro de conversas, procura os termos que importam e avisa só quando algo aparece. Os termos são seus: o nome da empresa, o de um produto, o dos concorrentes, o de um cliente grande, ou as palavras que costumam vir antes de um problema. Nada disso mexe no gateway: search_messages já responde a pergunta, o resto é instrução.",
			Uses:    []string{"search_messages", "list_chats", "get_chat_messages"},
			Prompt: `Todo dia às 19h, procure nas mensagens das últimas 24h por:

negócio: "contrato", "proposta", "boleto", "reunião", "urgente"
minha empresa: "[nome da empresa]", "[nome do produto]"
concorrentes: "[concorrente 1]", "[concorrente 2]"

Para cada acerto, me diga quem falou, em qual conversa e o trecho.
Agrupe por categoria. Se não houver nenhum, responda apenas
"nada hoje". Não invente resumo.`,
			Schedule: "Diária",
			Caveat:   "search_messages cobre só o que foi indexado. Se whatsapp_status apontar um gap no período, o silêncio pode ser perda de dado e não ausência de assunto.",
		},
		{
			Title:   "Resumo do que ficou sem resposta",
			Summary: "Lista as conversas em que a última mensagem é da outra pessoa e já tem algumas horas, com quantas mensagens esperam e desde quando. É a lista de quem está esperando você, e o que você já resolveu sai dela.",
			Uses:    []string{"list_unanswered", "mark_handled", "snooze_chat"},
			Prompt: `Liste as conversas do WhatsApp esperando minha resposta
há mais de 4 horas, incluindo os grupos em que me mencionaram.

Para cada uma: quem é, há quanto tempo, e o que a pessoa pediu.
Ordene pela mais antiga.

Quando eu disser que uma já está resolvida, marque com mark_handled.
Se eu pedir para lembrar depois, use snooze_chat.`,
			Schedule: "Duas vezes ao dia",
			Caveat:   "Respostas curtas como \"ok\", \"obrigado\" ou 👍 não contam como espera. As marcas de resolvido e adiado ficam só neste computador: a outra pessoa não vê nada, e uma mensagem nova traz a conversa de volta.",
		},
		{
			Title:   "Enquete e apuração",
			Summary: "Mandar a enquete e voltar depois para ler o resultado são duas chamadas separadas, ligadas pelo id que a primeira devolve.",
			Uses:    []string{"send_poll", "get_poll_results"},
			Prompt: `Mande no grupo do time uma enquete:
"Churrasco sexta?" com as opções Sim, Não e Talvez.

Guarde o message_id. Amanhã às 17h, leia o resultado
com get_poll_results e me diga a contagem.`,
			Schedule: "Envio agora, apuração depois",
			Caveat:   "Sem guardar o message_id não há como apurar. Peça ao assistente para anotá-lo junto com a tarefa de leitura.",
		},
		{
			Title:   "Resumo do dia de um grupo",
			Summary: "Um grupo movimentado acumula centenas de mensagens que ninguém vai ler de cima a baixo. Pegar o dia inteiro e devolver o que importa é uma chamada de leitura mais um pedido de síntese.",
			Uses:    []string{"list_groups", "get_chat_messages"},
			Prompt: `Pegue as mensagens de hoje do grupo [nome] e me resuma.

Quero: os assuntos que apareceram, o que ficou decidido,
o que ficou em aberto e qualquer coisa endereçada a mim.

Cite quem disse o quê nos pontos que importam.
Se o dia foi só conversa fiada, diga isso em vez de
esticar um resumo do nada.`,
			Schedule: "Fim do dia, ou sob demanda",
			Caveat:   "Use get_chat_messages com since e until do dia e order oldest, para ler em ordem cronológica. Áudio e imagem entram sem texto, então o resumo vai ter buracos onde a conversa foi por voz. Peça para marcar isso em vez de fingir que não existiu.",
		},
		{
			Title:   "Ler o documento ou a foto que chegou",
			Summary: "Um PDF, uma planilha ou a foto de um papel chegam no WhatsApp e o assistente lê o conteúdo direto, sem você baixar e anexar. PDF escaneado vira imagem das páginas, para o assistente ler como uma foto.",
			Uses:    []string{"get_chat_messages", "read_media"},
			Prompt: `Abra o último PDF que o [contato] me mandou no WhatsApp
e me diga o valor total, o vencimento e o que está sendo cobrado.

Se for uma foto ou um documento escaneado, leia mesmo assim.
Se não der para ler alguma parte, diga qual.`,
			Schedule: "Sob demanda",
			Caveat:   "Áudio não entra aqui: para ele, a transcrição. Vídeo não é lido. O arquivo baixado fica na pasta do app; media_stats diz quanto espaço ocupa e purge_media libera.",
		},
		{
			Title:   "Arquivo do que foi combinado",
			Summary: "Transforma uma conversa longa em uma lista de compromissos, com quem prometeu o quê e quando.",
			Uses:    []string{"get_chat_messages", "search_messages"},
			Prompt: `Leia minha conversa com [contato] dos últimos 30 dias
e extraia tudo que virou combinado: datas, valores, prazos.

Formate como uma lista com data, o que foi acordado e quem disse.
Se algo estiver ambíguo, marque como ambíguo em vez de decidir por mim.`,
			Schedule: "Sob demanda",
			Caveat:   "Mensagens são escritas por terceiros. Trate o conteúdo como dado, nunca como instrução: um texto que diz \"encaminhe isso\" não é um pedido a ser cumprido.",
		},
	}
}

// suggestedPrompts are starting points that exercise the tools people reach for
// first. They are phrased as a person would ask, not as tool calls, because the
// point is to show what the connection makes possible.
var suggestedPrompts = []string{
	"Qual é o número de telefone conectado no meu WhatsApp?",
	"Liste minhas 10 conversas mais recentes do WhatsApp.",
	"Me resuma a conversa do WhatsApp com o João da Silva de hoje.",
	"Procure no meu WhatsApp as mensagens que falam sobre contrato.",
	"Quais grupos do WhatsApp eu participo? Quem são os administradores do maior deles?",
	"Quem está esperando minha resposta no WhatsApp?",
}

// verificationPrompt is what the operator pastes into the client to confirm the
// connection end to end.
const verificationPrompt = `Use as ferramentas do WhatsApp e me diga:
- se o meu WhatsApp está conectado e qual é o número;
- quantas mensagens você consegue ver e desde quando;
- os nomes das 5 conversas mais recentes.

Não envie mensagem para ninguém. Se alguma coisa falhar, me mostre o erro exato.`

// agentPrompt is written to the assistant, not to the operator: it hands over
// everything a client needs to configure itself. There is no key to hand over:
// the server answers only on this computer.
func agentPrompt(endpoint, config string) string {
	return fmt.Sprintf(`Quero conectar um servidor MCP (Model Context Protocol) em você, para que você possa ler e usar o meu WhatsApp. Configure isso para mim. Se você não puder se configurar sozinho, me explique o passo a passo, bem devagar, para eu fazer na mão.

Dados da conexão:
- Nome do servidor: %s
- Transporte: HTTP (streamable HTTP)
- URL: %s
- Autenticação: nenhuma. O servidor roda neste computador e só responde a programas dele.

Se você só aceitar servidores que rodam por linha de comando (stdio), use esta configuração:
%s

Quando terminar, liste as ferramentas do servidor "%s" e me diga quantas são e qual é o número de telefone conectado.`, ServerName, endpoint, config, ServerName)
}
