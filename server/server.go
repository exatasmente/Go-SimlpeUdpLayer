package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/ioutil"
	"log"
	"math"
	"math/rand"
	"net"
	"strconv"
	"time"
	"unicode/utf8"
)

const maxBufferSize = 512

/*** REPOSITORY *****/
type User struct {
	Id       string    `json:"id"`
	Password string `json:"password"`
}

type UserRepository struct {
	users       []User
	storagePath string
	loaded      bool
}

func (repo *UserRepository) readFile() []User {
	if repo.loaded {
		return  repo.users
	}
	file, err := ioutil.ReadFile(repo.storagePath)
	if err != nil {
		fmt.Println(err.Error())
	}

	var users []User
	err = json.Unmarshal([]byte(file), &users)
	if err != nil {
		fmt.Println(err.Error())
	}
	repo.users = users
	repo.loaded = true

	return users
}

func (repo *UserRepository ) FindById(id string) (User, error) {
	var users = repo.readFile()

	for _, user := range users{
		if user.Id == id {
			return user , nil
		}
	}
	return User{}, errors.New("user not found")
}

func (repo *UserRepository ) save(user User )  (bool, error) {
	_ , err := repo.FindById(user.Id)
	if err != nil {

		repo.users = append(repo.users, user)
		usersJson, err := json.Marshal(repo.users)

		if err != nil {
			fmt.Println(err.Error())
		} else {
			err = ioutil.WriteFile(repo.storagePath, usersJson, 0644)
			if err != nil {
				fmt.Println(err.Error())
			}
			return true, nil
		}

	} else {
		panic("Usuario ja exixte")
	}

	return false, errors.New("user already exists")

}

func (repo *UserRepository) init()  {
	repo.users = make([]User, 0)
	repo.storagePath = "server/user.json"

	repo.readFile()
}
/***  END REPOSITORY *****/




/**
	// Package Flow Server Side:

	Socket -> Read -> Server -> Request -> | App  -
									               |
    Socket <- WriteTo <- Server <- Response  | < - -
 */

/********* UDP LAYER SERVER CORE ***************/
type Client struct {
	id string
	addr  net.Addr
	session string
	values map[string]string
	req chan Message
}

func (c *Client) setSession(s string) {
	c.session = s
}

func (c *Client) get(val string) (string, bool) {
	v := c.values[c.session+"_"+val]
	fmt.Println(v, val, len(v) > 0, c.values)
	return v, len(v) > 0
}

func (c *Client) set(key, val string) {
	c.values[c.session+"_"+key] = val
	fmt.Println(c.values, "VAL : " + val)
}

func (c *Client) remove(s string) {
	fmt.Println(s);
	delete(c.values, c.session+ "_" + s)
}

func (c *Client) message(msg string) {
	c.req <-  Message{
		Addr:   c.addr.String(),
		Action: "message",
		Status: 200,
		Data:   []byte(msg),
	}
}

type Message struct {
	Addr   string `json:"addr"`
	Action string `json:"action"`
	Status int32  `json:"status"`
	Data   []byte `json:"data"`
}

type action func (ctx context.Context ,request *Request)

type Server struct {
	clients  map[string]*Client
	socket   *net.UDPConn
	res      chan Message
	actions  map[string]action
	services context.Context
}

type Request struct {
	message *Message
	action string
	responseChan chan Message
	reqBody  string
}

func (req *Request) res(message string, status int32)  {

	req.responseChan <- Message{
		Addr:   req.message.Addr,
		Action: req.action,
		Status: status,
		Data:   []byte(message),
	}
}

func (s * Server) Serve(port int)  {
	var err error

	s.socket, err = net.ListenUDP("udp", &net.UDPAddr{IP: nil, Port: port})
	s.clients = make(map[string]*Client)
	s.res = make(chan Message)

	if err != nil {
		log.Fatal(err)
	}

	defer s.socket.Close()

	go func() {
		var data Message
		for {
			data = <-s.res
			if client, ok  := s.clients[data.Addr]; ok {
				fmt.Println(string(data.Data))
				_, err = s.socket.WriteTo(data.Data, client.addr)

				if err != nil {
					fmt.Errorf(err.Error())
					delete(s.clients, data.Addr)
				}

			}
		}
	}()

	for {
		buf := make([]byte, maxBufferSize)
		n, addr, err := s.socket.ReadFrom(buf)
		if err != nil {
			continue
		}

		message := Message{}
		err = json.Unmarshal(buf[:n], &message)
		fmt.Println(message)
		if err != nil {
			fmt.Println(err.Error())
		}
		message.Addr = addr.String()

		if client, ok  := s.clients[addr.String()]; ok {
			go s.handleRequest(message, client)
		} else {
			go s.newConnection(addr)
		}
	}

}

func (s *Server) newConnection(addr net.Addr)  {
	client := &Client{
		session: "new-connection",
		addr: addr,
		req: s.res,
		values: make(map[string]string),
	}

	s.clients[addr.String()] = client
	message := Message{
		Addr:   addr.String(),
		Action: "init",
		Status: 200,
	}
	s.handleRequest(message, client)

}


func (s *Server) handleRequest(message Message, client *Client)  {
	var reqBody = string(message.Data)

	request := Request{
		message: &message,
		action:  client.session,
		reqBody: reqBody,
		responseChan: s.res,
	}

	s.callAction(request.action, request, client)
}

func (s *Server) callAction(action string, request Request, client *Client) {
	fmt.Println(client.session)
	s.actions[action](s.getContext(request, client), &request)
}

func (s *Server) getContext(request Request, client *Client) context.Context {
	return context.WithValue(s.services, "client", client)
}

/********* END UDP LAYER SERVER CORE ***************/

/****** APP *********/

func newConnection(ctx context.Context, request *Request)  {
	var client *Client
	client = ctx.Value("client").(*Client)
	client.setSession("home")
	request.res("Bem vindo ao servidor\n ------------------ \n |(1)->Entrar \n |(2) ->Cadastre-se\n", 200)
}


func home(ctx context.Context, request *Request) {
	option := request.reqBody
	if option == "1" {
		login(ctx, request)
		return
	}

	if option == "2" {
		signup(ctx, request)
		return
	}

}

func login(ctx context.Context, request *Request)  {
	var client *Client
	var id string

	client = ctx.Value("client").(*Client)

	if client.session != "login" {
		client.setSession("login")
		request.res("---------------------LOGIN ------------------ \n Qual a seu ID ? \n", 200)
		return
	}

	id, ok := client.get("id")

	if  !ok {
		fmt.Println(request)
		client.set("id", request.reqBody)
		request.res("---------------------LOGIN ------------------ \n Qual a sua senha ? \n", 200)
		return
	}

	users := ctx.Value("User").(*UserRepository)

	user, err := users.FindById(id)

	fmt.Println(user, request.reqBody)
	if err != nil {
		request.res(err.Error(), 400)
	} else if  user.Password == request.reqBody {
		client.id = id
		client.remove("id")
		mainMenu(ctx, request)
	} else {
		client.remove("id")
		client.setSession("new-connection")
		request.res("----------------------------------\n Usuario Invalido\n-------------------------------------\n", 200)
	}

}

func  signup(ctx context.Context, request *Request)  {
	var client *Client
	var id string

	client = ctx.Value("client").(*Client)

	if client.session != "signup" {
		client.setSession("signup")
		request.res("--------------------- CADASTRO ------------------ \n Informe um ID \n", 200)
		return
	}

	id, ok := client.get("id")

	if  !ok {
		fmt.Println(request)
		client.set("id", request.reqBody)
		request.res("--------------------- CADASTRO ------------------ \n Informe uma senha \n", 200)
		return
	}

	users := ctx.Value("User").(*UserRepository)
	_, err := users.FindById(id)
	if err != nil {
		user := User{
			Id:       id,
			Password: request.reqBody,
		}
		err = nil

		_, err := users.save(user)

		if err != nil {
			client.remove("id")
			client.setSession("new-connection")
			request.res(err.Error(), 500)
		} else {
			client.id = id
			client.remove("id")
			mainMenu(ctx, request)
		}

	} else {
		fmt.Println(client)
		client.remove("id")
		fmt.Println(client)
		client.setSession("new-connection")
		request.res("----------------------------------\n ID JA CADASTRADO\n-------------------------------------\n", 200)
	}
}


func mainMenu(ctx context.Context, request *Request)  {
	var client *Client
	client = ctx.Value("client").(*Client)


	if client.session != "main-menu" {
		client.setSession("main-menu")
		request.res("----------------------------------\n |(1)-> Inverter String \n|(2)-> IMC\n|(3)-> Número Aleatório \n|(4)-> Mensagem direta \n|(5)->Chat \n|(6)-> Cadastrar filme \n|(7)-> Filme Aleatório \n|(8)-> Sair", 200)
		return
	}

	switch request.reqBody {
	case "1":
		strInverse(ctx, request)
		break
	case "2":
		imc(ctx, request)
		break
	case "3":
		randomNumber(ctx, request)
		break
	case "4":
		directMessage(ctx, request)
		break
	case "5":
		chat(ctx, request)
		break
	case "6":
		storeMovie(ctx, request)
		break
	case "7":
		findMovie(ctx, request)
		break

	}

}

func imc(ctx context.Context, request *Request) {
	var client *Client

	client = ctx.Value("client").(*Client)

	if client.session != "imc" {
		client.setSession("imc")
		request.res("--------------------- IMC ------------------ \n Informe o peso em quilos \n", 200)
		return
	}

	weightStr, ok := client.get("weight")

	if  !ok {
		fmt.Println(request)
		client.set("weight", request.reqBody)
		request.res("--------------------- IMC ------------------ \n Informe a Altura em metros \n", 200)
		return
	}

	height, err := strconv.ParseFloat(request.reqBody, 64)
	if err != nil {
		request.res("--------------------- IMC ------------------ \n Altura invalida  \n", 400)
		mainMenu(ctx,request)
		return
	}

	weight, err := strconv.ParseFloat(weightStr, 64)
	if err != nil {
		request.res("--------------------- IMC ------------------ \n Peso invalido \n", 400)
		mainMenu(ctx,request)
		return
	}

	imc := weight / math.Pow(height, 2)

	var situation string

	if imc < 17 {
		situation = "Muito abaixo do peso"
	} else if imc >= 17 || imc <= 18.49 {
		situation = "Abaixo do peso"
	} else if imc >= 18.5 || imc <= 24.99 {
		situation = "Peso normal"
	} else if imc >= 25 || imc <= 29.99 {
		situation = "Acima do peso"
	} else if imc >= 30 || imc <= 34.99 {
		situation = "Obesidade I"
	} else if imc >= 35 || imc <= 39.99 {
		situation = "Obesidade II (severa)"
	} else if imc >= 35 || imc <= 39.99 {
		situation = "Obesidade III (mórbida)"
	}

	request.res(situation, 200)
	client.remove("weight")
	mainMenu(ctx, request)
}

func  strInverse(ctx context.Context, request *Request)  {
	var client *Client
	client = ctx.Value("client").(*Client)

	if client.session != "str-inverse" {
		client.setSession("str-inverse")
		request.res("----------------------------------\n INVERTER STRING ----------------------------------------\n Informe a string: ", 200)
		return
	}

	if request.reqBody == "SAIR" {
		mainMenu(ctx, request)
		return
	}

	size := len(request.reqBody)
	buf := make([]byte, size)
	for start := 0; start < size; {
		r, n := utf8.DecodeRuneInString(request.reqBody[start:])
		start += n
		utf8.EncodeRune(buf[size-start:], r)
	}
	request.res(string(buf), 200)
}


func directMessage(ctx context.Context, request *Request)  {

	var client *Client
	client = ctx.Value("client").(*Client)

	if client.session != "direct-message" {
		client.setSession("direct-message")
		request.res("----------------------------------\nMENSAGEM DIRETA\n----------------------------------------\n Informe o id do usuario: ", 200)
		return
	}

	id, ok := client.get("id")

	if  !ok {
		fmt.Println(request)
		if request.reqBody == client.id {
			request.res("----------------------------------\nMENSAGEM DIRETA\n----------------------------------------\n ID invalido! ", 200)
			mainMenu(ctx, request)
		}

		client.set("id", request.reqBody)

		request.res("----------------------------------\nMENSAGEM DIRETA\n----------------------------------------\n digite a mensagem: ", 200)
		return
	}


	msg := request.reqBody
	online := ctx.Value("clients").(*map[string]*Client)

	fmt.Println(online)
	sent := false
	for _,user := range *online {
		fmt.Println(user)
		if user.id == id {
			user.message(msg)
			sent = true
			break
		}
	}

	if sent {
		request.res("----------------------------------\nMENSAGEM DIRETA\n----------------------------------------\n Mensagem enviada com sucesso! ", 200)
	} else {
		request.res("----------------------------------\nMENSAGEM DIRETA\n----------------------------------------\n Falha no envio da mensagem! ", 200)
	}
	client.remove("id")
	mainMenu(ctx, request)

}

func chat(ctx context.Context, request *Request)  {
	var client *Client
	client = ctx.Value("client").(*Client)

	if client.session != "chat" {
		client.setSession("chat")
		request.res("----------------------------------\nCHAT\n----------------------------------------\n", 200)
		return
	}

	if request.reqBody == "SAIR" {
		mainMenu(ctx, request)
		return
	}

	online := ctx.Value("clients").(*map[string]*Client)

	for _,user := range *online {
		if user.id !=  client.id && user.session == "chat" {
			user.message("#" + client.id + ": " +request.reqBody)
		}
	}

}

func randomNumber(ctx context.Context, request *Request)  {
	var client *Client
	client = ctx.Value("client").(*Client)

	if client.session != "random-number" {
		client.setSession("random-number")
		request.res("----------------------------------\nNUMERO ALEATORIO\n----------------------------------------\nInforme o valor minimo", 200)
		return
	}

	minStr, ok := client.get("min")
	if  !ok {
		fmt.Println(request)
		client.set("min", request.reqBody)

		request.res("----------------------------------\nNUMERO ALEATORIO\n----------------------------------------\nInforme o valor maximo: ", 200)
		return
	}
	rand.Seed(time.Now().UnixNano())

	min, _ := strconv.Atoi(minStr)
	max, _ := strconv.Atoi(request.reqBody)
	val := rand.Intn((max - min + 1) + min)

	request.res(strconv.Itoa(val),200)
	client.setSession("main-menu")

}

func storeMovie(ctx context.Context, request *Request)  {
	request.res("----------------------------------\nOPÇAO INDISPONIVEL\n----------------------------------------\n", 200)
	mainMenu(ctx, request)
}

func findMovie(ctx context.Context, request *Request)  {
	request.res("----------------------------------\nOPÇAO INDISPONIVEL\n----------------------------------------\n", 200)
	mainMenu(ctx, request)
}
/****** END APP *********/


func main() {
	userRepository := UserRepository{}
	userRepository.init()

	server := Server{
		clients: nil,
		socket:    nil,
		res:  make(chan Message),
		actions: map[string]action{
			"home" : home,
			"new-connection" : newConnection,
			"main-menu" : mainMenu,
			"login" : login,
			"signup" : signup,
			"str-inverse" : strInverse,
			"imc" : imc,
			"random-number" : randomNumber,
			"direct-message" : directMessage,
			"chat" : chat,
			"store-movie" : storeMovie,
			"find-movie" : findMovie,
		},
	}

	server.services = context.WithValue(context.Background(), "User", &userRepository)
	server.services = context.WithValue(server.services, "clients", &server.clients)
	server.Serve(1054)

}
