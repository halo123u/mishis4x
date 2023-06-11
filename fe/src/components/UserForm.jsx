const UserForm = (props) => {
  const handleSubmit = (event) => {
    event.preventDefault()
    props.submit(event.target.username.value, event.target.password.value)
  }

  return (
    <form onSubmit={handleSubmit}>
      <label htmlFor="username">Username</label>
      <input type="text" name="username" id="username" />
      <label htmlFor="password">Password</label>
      <input type="password" name="password" id="password" />

      <button type="submit">{props.buttonText}</button>
    </form>
  )
}

export default UserForm
