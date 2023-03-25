const CreateUserForm = (props) => {
  const handleSubmit = (event) => {
    event.preventDefault()
    props.createUser(event.target.username.value, event.target.password.value)
  }

  return (
    <form onSubmit={handleSubmit}>
      <label htmlFor="username" >Username</label>
      <input type="text" name="username" id="username" />
      <label htmlFor="password">Password</label>
      <input type="password" name="password" id="password" />

      <button type="submit">Create User</button>
    </form>
  )
}

export default CreateUserForm