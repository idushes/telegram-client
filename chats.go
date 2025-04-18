package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/gotd/td/telegram"
	"github.com/gotd/td/telegram/auth"
	"github.com/gotd/td/tg"
)

// ChatInfo содержит информацию о чате
type ChatInfo struct {
	ID        int64  `json:"id"`
	Title     string `json:"title"`
	Type      string `json:"type"`
	Username  string `json:"username,omitempty"`
	Members   int    `json:"members,omitempty"`
	IsBot     bool   `json:"is_bot,omitempty"`
	FirstName string `json:"first_name,omitempty"`
	LastName  string `json:"last_name,omitempty"`
}

// ChatsResponse содержит список чатов для вывода в JSON
type ChatsResponse struct {
	Chats []ChatInfo `json:"chats"`
	Count int        `json:"count"`
}

// GetChats получает список всех доступных чатов
func GetChats(ctx context.Context, config AuthConfig) error {
	// Create client
	client := telegram.NewClient(config.AppID, config.AppHash, telegram.Options{
		SessionStorage: &telegram.FileSessionStorage{
			Path: config.SessionFile,
		},
	})

	// Канал для передачи результата или ошибки
	resultCh := make(chan *ChatsResponse, 1)
	errCh := make(chan error, 1)

	// Запускаем клиент
	go func() {
		err := client.Run(ctx, func(ctx context.Context) error {
			// Авторизируемся при необходимости
			flow := auth.NewFlow(
				auth.CodeOnly(config.Phone, &telegramCodeAuth{}),
				auth.SendCodeOptions{},
			)

			// Выполняем авторизацию если нужно
			fmt.Println("Checking authorization...")
			if err := client.Auth().IfNecessary(ctx, flow); err != nil {
				return fmt.Errorf("authentication error: %w", err)
			}

			// Проверяем авторизацию
			status, err := client.Auth().Status(ctx)
			if err != nil {
				return fmt.Errorf("failed to get auth status: %w", err)
			}

			if !status.Authorized {
				return fmt.Errorf("not authorized")
			}

			fmt.Println("Getting chats...")
			// Получаем все диалоги
			dialogsClass, err := client.API().MessagesGetDialogs(ctx, &tg.MessagesGetDialogsRequest{
				OffsetPeer: &tg.InputPeerEmpty{},
				Limit:      100, // ограничиваем количество чатов
			})
			if err != nil {
				return fmt.Errorf("failed to get dialogs: %w", err)
			}

			// Преобразуем полученные данные
			dialogs, err := extractChats(dialogsClass)
			if err != nil {
				return fmt.Errorf("failed to extract chats: %w", err)
			}

			// Отправляем результат
			resultCh <- dialogs
			return nil
		})

		if err != nil {
			errCh <- err
		}
	}()

	// Ждем или результат, или ошибку, или таймаут
	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-errCh:
		return err
	case result := <-resultCh:
		// Выводим результат в формате JSON
		jsonData, err := json.MarshalIndent(result, "", "  ")
		if err != nil {
			return fmt.Errorf("failed to convert to JSON: %w", err)
		}
		fmt.Println(string(jsonData))
		return nil
	case <-time.After(2 * time.Minute): // Таймаут 2 минуты
		return fmt.Errorf("operation timed out")
	}
}

// extractChats извлекает информацию о чатах из ответа API
func extractChats(dialogsClass tg.MessagesDialogsClass) (*ChatsResponse, error) {
	var dialogs []tg.DialogClass
	var chats []tg.ChatClass
	var users []tg.UserClass

	// Извлекаем данные в зависимости от типа полученного ответа
	switch d := dialogsClass.(type) {
	case *tg.MessagesDialogs:
		dialogs = d.Dialogs
		chats = d.Chats
		users = d.Users
	case *tg.MessagesDialogsSlice:
		dialogs = d.Dialogs
		chats = d.Chats
		users = d.Users
	default:
		return nil, fmt.Errorf("unexpected type of dialogs: %T", dialogsClass)
	}

	// Создаем карты для быстрого доступа к чатам и пользователям по ID
	chatMap := make(map[int64]tg.ChatClass)
	userMap := make(map[int64]tg.UserClass)

	// Заполняем карту чатов
	for _, c := range chats {
		switch chat := c.(type) {
		case *tg.Chat:
			chatMap[chat.GetID()] = chat
		case *tg.Channel:
			chatMap[chat.GetID()] = chat
		}
	}

	// Заполняем карту пользователей
	for _, u := range users {
		if user, ok := u.(*tg.User); ok {
			userMap[user.GetID()] = user
		}
	}

	// Создаем результат
	result := &ChatsResponse{
		Chats: make([]ChatInfo, 0, len(dialogs)),
	}

	// Обрабатываем каждый диалог
	for _, dialogClass := range dialogs {
		// Проверяем, что это базовый диалог (не папка)
		dialog, ok := dialogClass.(*tg.Dialog)
		if !ok {
			continue
		}

		var info ChatInfo

		// Определяем тип пира (чата)
		switch peer := dialog.Peer.(type) {
		case *tg.PeerUser:
			// Пропускаем диалоги с пользователями (тип "user")
			continue
		case *tg.PeerChat:
			// Это групповой чат
			if chat, ok := chatMap[peer.ChatID]; ok {
				if c, ok := chat.(*tg.Chat); ok {
					info = ChatInfo{
						ID:      c.GetID(),
						Title:   c.Title,
						Type:    "chat",
						Members: int(c.ParticipantsCount),
					}
				}
			}
		case *tg.PeerChannel:
			// Это канал или супергруппа
			if channel, ok := chatMap[peer.ChannelID]; ok {
				if c, ok := channel.(*tg.Channel); ok {
					channelType := "channel"
					if c.Megagroup {
						channelType = "supergroup"
					}
					info = ChatInfo{
						ID:       c.GetID(),
						Title:    c.Title,
						Type:     channelType,
						Username: c.Username,
						Members:  int(c.ParticipantsCount),
					}
				}
			}
		}

		// Добавляем информацию в результат
		if info.ID != 0 {
			result.Chats = append(result.Chats, info)
		}
	}

	result.Count = len(result.Chats)
	return result, nil
}
