-- name: ListPosts :many
SELECT 
  id
  , created
  , title
  , body 
FROM posts 
ORDER BY id DESC;

-- name: GetPost :one
SELECT 
  id
  , created
  , title
  , body 
FROM posts 
WHERE id = ?;

-- name: CreatePost :exec
INSERT INTO posts(title, body)
VALUES(?, ?);
